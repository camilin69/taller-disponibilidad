package httpin

import (
	"log"
	"math/rand"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
)

// Replica es el adaptador HTTP de un proceso replica: responde el eco del
// Ping/Echo, entrega el saldo y ofrece el gancho de caos del inyector.
type Replica struct {
	id          string
	saldoBase   int64
	latenciaMin time.Duration
	latenciaMax time.Duration

	// Salir se invoca en POST /chaos/crash; se inyecta para poder probar el
	// endpoint sin terminar el proceso de pruebas.
	Salir func(codigo int)

	pings     atomic.Int64
	consultas atomic.Int64
}

// NuevaReplica construye el adaptador de una replica.
func NuevaReplica(id string, saldoBase int64, latenciaMin, latenciaMax time.Duration) *Replica {
	return &Replica{
		id:          id,
		saldoBase:   saldoBase,
		latenciaMin: latenciaMin,
		latenciaMax: latenciaMax,
		Salir:       func(codigo int) { log.Printf("replica %s: crash solicitado (codigo %d)", id, codigo) },
	}
}

// Rutas construye el enrutador de la replica.
func (rep *Replica) Rutas() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", rep.manejarPing)
	mux.HandleFunc("/saldo/", rep.manejarSaldo)
	mux.HandleFunc("/chaos/crash", rep.manejarCrash)
	mux.HandleFunc("/salud", rep.manejarSalud)
	return ConCORS(mux)
}

// manejarPing es el ECO del Ping/Echo: 200 con el identificador de la replica.
func (rep *Replica) manejarPing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		escribirError(w, http.StatusMethodNotAllowed, "metodo no permitido", r.Method)
		return
	}
	rep.pings.Add(1)
	escribirJSON(w, http.StatusOK, map[string]string{"replica_id": rep.id})
}

// manejarSaldo devuelve el saldo de la tarjeta con una latencia artificial
// aleatoria: asi ninguna replica gana siempre y se evidencia la competencia de
// la redundancia activa (experimento E0).
func (rep *Replica) manejarSaldo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		escribirError(w, http.StatusMethodNotAllowed, "metodo no permitido", r.Method)
		return
	}
	idTarjeta := segmentoFinal(r.URL.Path, "/saldo")
	if err := domain.ValidarIDTarjeta(idTarjeta); err != nil {
		escribirError(w, http.StatusBadRequest, "id de tarjeta invalido", err.Error())
		return
	}
	rep.consultas.Add(1)

	if espera := rep.latenciaSimulada(); espera > 0 {
		select {
		case <-time.After(espera):
		case <-r.Context().Done(): // el dispatcher ya tiene ganador: se descarta
			return
		}
	}

	escribirJSON(w, http.StatusOK, domain.Saldo{
		ReplicaID: rep.id,
		IDTarjeta: idTarjeta,
		Valor:     domain.CalcularSaldo(rep.saldoBase, idTarjeta),
	})
}

// manejarCrash termina el proceso: simula el crash de la replica. Lo usa
// unicamente el inyector de fallas.
func (rep *Replica) manejarCrash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		escribirError(w, http.StatusMethodNotAllowed, "solo POST", r.Method)
		return
	}
	log.Printf("[CAOS] replica %s: recibida orden de crash a las %s",
		rep.id, time.Now().UTC().Format(domain.FormatoTimestamp))
	escribirJSON(w, http.StatusOK, map[string]string{
		"replica_id": rep.id,
		"mensaje":    "terminando el proceso",
		"momento":    time.Now().UTC().Format(domain.FormatoTimestamp),
	})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	// Se termina en otra goroutine para alcanzar a enviar la respuesta.
	go func() {
		time.Sleep(50 * time.Millisecond)
		rep.Salir(137) // 137 = terminado por senal, igual que docker kill
	}()
}

// manejarSalud expone contadores basicos del proceso replica.
func (rep *Replica) manejarSalud(w http.ResponseWriter, r *http.Request) {
	escribirJSON(w, http.StatusOK, map[string]any{
		"servicio":   "replica",
		"replica_id": rep.id,
		"pings":      rep.pings.Load(),
		"consultas":  rep.consultas.Load(),
	})
}

// latenciaSimulada sortea una latencia uniforme en [min, max].
func (rep *Replica) latenciaSimulada() time.Duration {
	if rep.latenciaMax <= 0 {
		return 0
	}
	rango := rep.latenciaMax - rep.latenciaMin
	if rango <= 0 {
		return rep.latenciaMin
	}
	return rep.latenciaMin + time.Duration(rand.Int63n(int64(rango)+1))
}
