package httpin

import (
	"errors"
	"net/http"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
	"github.com/camilin69/taller-disponibilidad/internal/usecase"
)

// ConfigExpuesta son los parametros del monitor que se publican para que la
// interfaz y la documentacion muestren con que T, t, k y m se esta corriendo.
type ConfigExpuesta struct {
	IntervaloSondeoMS   int64 `json:"T_intervalo_ms"`
	TimeoutSondeoMS     int64 `json:"t_timeout_ms"`
	K                   int   `json:"k_fallos_para_caida"`
	M                   int   `json:"m_exitos_para_viva"`
	TimeoutConsultaMS   int64 `json:"timeout_consulta_ms"`
	DeteccionEsperadaMS int64 `json:"deteccion_esperada_ms"`
}

// RespuestaEstadoDetalle es el cuerpo de GET /estado/detalle (panel React).
type RespuestaEstadoDetalle struct {
	Momento  time.Time                   `json:"momento"`
	Replicas []usecase.SaludReplica      `json:"replicas"`
	Config   ConfigExpuesta              `json:"config"`
	Metricas usecase.ResumenEstadisticas `json:"metricas"`
	Bitacora []string                    `json:"bitacora"`
}

// RespuestaSaldo es el cuerpo de GET /saldo/{id} del dispatcher.
type RespuestaSaldo struct {
	ReplicaID      string   `json:"replica_id"`
	IDTarjeta      string   `json:"id_tarjeta"`
	Saldo          int64    `json:"saldo"`
	LatenciaMS     float64  `json:"latencia_ms"`
	ReplicasUsadas []string `json:"replicas_consultadas"`
}

// Dispatcher es el adaptador HTTP de entrada del dispatcher: unico punto de
// entrada del cliente y ventana de observacion del monitor.
type Dispatcher struct {
	consulta *usecase.ConsultarSaldo
	registro *usecase.RegistroReplicas
	bitacora usecase.Bitacora
	config   ConfigExpuesta

	// Registro controla la traza de peticiones atendidas. Lo fija la capa de
	// infraestructura a partir de LOG_PETICIONES.
	Registro OpcionesRegistro
}

// NuevoDispatcher arma el adaptador con sus dependencias ya construidas.
func NuevoDispatcher(consulta *usecase.ConsultarSaldo, registro *usecase.RegistroReplicas,
	bitacora usecase.Bitacora, config ConfigExpuesta) *Dispatcher {
	return &Dispatcher{consulta: consulta, registro: registro, bitacora: bitacora, config: config}
}

// Rutas construye el enrutador del dispatcher.
func (d *Dispatcher) Rutas() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/saldo/", d.manejarSaldo)
	mux.HandleFunc("/estado", d.manejarEstado)
	mux.HandleFunc("/estado/detalle", d.manejarEstadoDetalle)
	mux.HandleFunc("/bitacora", d.manejarBitacora)
	mux.HandleFunc("/metricas", d.manejarMetricas)
	mux.HandleFunc("/salud", d.manejarSalud)
	// El registro va por fuera de CORS para que tambien queden trazadas las
	// peticiones de verificacion previa (OPTIONS) del panel React.
	return ConRegistroPeticiones(ConCORS(mux), d.Registro)
}

// manejarSaldo atiende la consulta del cliente con redundancia activa (R2).
func (d *Dispatcher) manejarSaldo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		escribirError(w, http.StatusMethodNotAllowed, "metodo no permitido", r.Method)
		return
	}
	idTarjeta := segmentoFinal(r.URL.Path, "/saldo")
	if err := domain.ValidarIDTarjeta(idTarjeta); err != nil {
		escribirError(w, http.StatusBadRequest, "id de tarjeta invalido", err.Error())
		return
	}

	resultado, err := d.consulta.Ejecutar(r.Context(), idTarjeta)
	if err != nil {
		codigo := http.StatusServiceUnavailable
		switch {
		case errors.Is(err, domain.ErrSinReplicasVivas):
			escribirError(w, codigo, "sin replicas VIVA", err.Error())
		case errors.Is(err, domain.ErrTodasFallaron):
			escribirError(w, codigo, "ninguna replica respondio", err.Error())
		default:
			escribirError(w, http.StatusBadGateway, "error consultando el saldo", err.Error())
		}
		return
	}

	// Encabezados utiles para el CSV del cliente sin tener que parsear el cuerpo.
	w.Header().Set("X-Replica-Id", resultado.Saldo.ReplicaID)
	escribirJSON(w, http.StatusOK, RespuestaSaldo{
		ReplicaID:      resultado.Saldo.ReplicaID,
		IDTarjeta:      idTarjeta,
		Saldo:          resultado.Saldo.Valor,
		LatenciaMS:     float64(resultado.Latencia.Microseconds()) / 1000.0,
		ReplicasUsadas: resultado.ReplicasUsadas,
	})
}

// manejarEstado devuelve el contrato exigido por el taller:
// {"A":"VIVA","B":"CAIDA"}.
func (d *Dispatcher) manejarEstado(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		escribirError(w, http.StatusMethodNotAllowed, "metodo no permitido", r.Method)
		return
	}
	escribirJSON(w, http.StatusOK, d.registro.Instantanea())
}

// manejarEstadoDetalle entrega el estado enriquecido para la interfaz React.
func (d *Dispatcher) manejarEstadoDetalle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		escribirError(w, http.StatusMethodNotAllowed, "metodo no permitido", r.Method)
		return
	}
	var lineas []string
	if d.bitacora != nil {
		lineas = d.bitacora.Ultimas(parametroEntero(r, "bitacora", 20))
	}
	escribirJSON(w, http.StatusOK, RespuestaEstadoDetalle{
		Momento:  time.Now(),
		Replicas: d.registro.Detalle(),
		Config:   d.config,
		Metricas: d.consulta.Estadisticas.Instantanea(),
		Bitacora: lineas,
	})
}

// manejarBitacora expone las ultimas lineas de la bitacora del monitor.
func (d *Dispatcher) manejarBitacora(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		escribirError(w, http.StatusMethodNotAllowed, "metodo no permitido", r.Method)
		return
	}
	var lineas []string
	if d.bitacora != nil {
		lineas = d.bitacora.Ultimas(parametroEntero(r, "n", 100))
	}
	escribirJSON(w, http.StatusOK, map[string]any{"lineas": lineas})
}

// manejarMetricas expone los contadores agregados de la redundancia activa.
func (d *Dispatcher) manejarMetricas(w http.ResponseWriter, r *http.Request) {
	escribirJSON(w, http.StatusOK, d.consulta.Estadisticas.Instantanea())
}

// manejarSalud indica que el proceso dispatcher esta en pie.
func (d *Dispatcher) manejarSalud(w http.ResponseWriter, r *http.Request) {
	escribirJSON(w, http.StatusOK, map[string]any{"servicio": "dispatcher", "estado": "OK"})
}
