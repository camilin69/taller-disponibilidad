package httpin

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync/atomic"

	"github.com/camilin69/taller-desempeno/internal/domain"
	"github.com/camilin69/taller-desempeno/internal/usecase"
)

// ConfigExpuesta son los parametros del taller que GET /config publica junto a
// los contadores en vivo, para poder verificar con que valores corre el
// experimento sin entrar al contenedor.
type ConfigExpuesta struct {
	CostoCalculoMS   int64 `json:"costo_calculo_ms"`
	ViajesPorDefecto int   `json:"viajes_por_defecto"`
}

// respuestaConfig es el cuerpo de GET /config. Compone los contadores en vivo
// de las dos tacticas mas los parametros del experimento; al embeber los
// estados, sus campos JSON se promueven a un unico objeto plano.
type respuestaConfig struct {
	usecase.EstadoPool  // R1: Introducir Concurrencia
	usecase.EstadoCache // R2: Mantener Multiples Copias de Datos
	ConfigExpuesta
}

// API es el adaptador HTTP de entrada del servidor de resumenes.
type API struct {
	uc  *usecase.ObtenerResumen
	cfg ConfigExpuesta

	// Registro controla la traza de peticiones atendidas.
	Registro OpcionesRegistro

	atendidas  atomic.Int64
	rechazadas atomic.Int64
}

// NuevaAPI construye el adaptador HTTP del servidor.
func NuevaAPI(uc *usecase.ObtenerResumen, cfg ConfigExpuesta) *API {
	return &API{uc: uc, cfg: cfg}
}

// Rutas construye el enrutador del servidor.
func (a *API) Rutas() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/resumen/", a.manejarResumen)
	mux.HandleFunc("/config", a.manejarConfig)
	mux.HandleFunc("/salud", a.manejarSalud)
	return ConRegistroPeticiones(ConCORS(mux), a.Registro)
}

// manejarResumen atiende GET /resumen/{idTarjeta}?viajes=N.
func (a *API) manejarResumen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		escribirError(w, http.StatusMethodNotAllowed, "metodo no permitido", r.Method)
		return
	}

	idTarjeta := segmentoFinal(r.URL.Path, "/resumen")
	if err := domain.ValidarIDTarjeta(idTarjeta); err != nil {
		escribirError(w, http.StatusBadRequest, "id de tarjeta invalido", err.Error())
		return
	}
	viajes := parametroEntero(r, "viajes", a.cfg.ViajesPorDefecto)
	if err := domain.ValidarViajes(viajes); err != nil {
		escribirError(w, http.StatusBadRequest, "numero de viajes invalido", err.Error())
		return
	}

	clave := domain.ClaveResumen{IDTarjeta: idTarjeta, Viajes: viajes}
	resumen, traza, err := a.uc.Ejecutar(r.Context(), clave)

	switch {
	case errors.Is(err, domain.ErrColaLlena):
		// Evento no procesado: el servidor esta saturado y lo dice de
		// inmediato en vez de dejar al cliente esperando hasta su timeout.
		a.rechazadas.Add(1)
		w.Header().Set("Retry-After", "1")
		escribirError(w, http.StatusServiceUnavailable, "servidor saturado", err.Error())
		return

	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		// El cliente se rindio (timeout de 3 s): ya nadie lee la respuesta.
		// Escribir aqui solo llenaria el log de errores falsos.
		return

	case err != nil:
		escribirError(w, http.StatusInternalServerError, "no se pudo calcular el resumen", err.Error())
		return
	}

	a.atendidas.Add(1)
	// La cabecera deja el origen de la respuesta visible para la traza de
	// peticiones y para curl, sin tener que leer el cuerpo.
	w.Header().Set("X-Desde-Cache", strconv.FormatBool(traza.DesdeCache))
	escribirJSON(w, http.StatusOK, resumen)
}

// manejarConfig publica cuantos trabajadores estan activos y cuantas entradas
// tiene el cache: es la verificacion en vivo de que R1 y R2 funcionan.
func (a *API) manejarConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		escribirError(w, http.StatusMethodNotAllowed, "metodo no permitido", r.Method)
		return
	}

	escribirJSON(w, http.StatusOK, respuestaConfig{
		EstadoPool:     a.uc.EstadoPool(),
		EstadoCache:    a.uc.EstadoCache(),
		ConfigExpuesta: a.cfg,
	})
}

// manejarSalud expone contadores basicos del proceso.
func (a *API) manejarSalud(w http.ResponseWriter, r *http.Request) {
	escribirJSON(w, http.StatusOK, map[string]any{
		"servicio":   "servidor-resumen",
		"atendidas":  a.atendidas.Load(),
		"rechazadas": a.rechazadas.Load(),
	})
}
