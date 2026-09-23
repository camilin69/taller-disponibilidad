package httpin

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
	"github.com/camilin69/taller-disponibilidad/internal/usecase"
)

// registradorDePrueba captura las lineas de log en memoria.
func registradorDePrueba() (*log.Logger, *bytes.Buffer) {
	var salida bytes.Buffer
	return log.New(&salida, "", 0), &salida
}

func TestRegistroDejaUnaLineaPorPeticion(t *testing.T) {
	registrador, salida := registradorDePrueba()
	manejador := ConRegistroPeticiones(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Replica-Id", "C")
		_, _ = w.Write([]byte(`{"saldo":15000}`))
	}), OpcionesRegistro{Activo: true, Registrador: registrador})

	peticion := httptest.NewRequest(http.MethodGet, "/saldo/1234", nil)
	peticion.RemoteAddr = "172.18.0.1:54321"
	manejador.ServeHTTP(httptest.NewRecorder(), peticion)

	linea := salida.String()
	for _, esperado := range []string{"GET", "/saldo/1234", "200", "replica=C", "cliente=172.18.0.1"} {
		if !strings.Contains(linea, esperado) {
			t.Fatalf("la traza no contiene %q: %q", esperado, linea)
		}
	}
}

func TestRegistroConservaElCodigoDeError(t *testing.T) {
	registrador, salida := registradorDePrueba()
	manejador := ConRegistroPeticiones(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		escribirError(w, http.StatusServiceUnavailable, "sin replicas VIVA", "")
	}), OpcionesRegistro{Activo: true, Registrador: registrador})

	grabadora := httptest.NewRecorder()
	manejador.ServeHTTP(grabadora, httptest.NewRequest(http.MethodGet, "/saldo/1234", nil))

	if grabadora.Code != http.StatusServiceUnavailable {
		t.Fatalf("el middleware altero el codigo: %d", grabadora.Code)
	}
	if !strings.Contains(salida.String(), "503") {
		t.Fatalf("la traza no registro el 503: %q", salida.String())
	}
}

// Por omision los sondeos del monitor no se trazan: son uno por segundo y por
// replica, y ahogarian el resto del log.
func TestRegistroOmiteTraficoDeVigilanciaSalvoQueSePida(t *testing.T) {
	registrador, salida := registradorDePrueba()
	opciones := OpcionesRegistro{Activo: true, Registrador: registrador}
	manejador := ConRegistroPeticiones(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"replica_id":"A"}`))
	}), opciones)

	for _, ruta := range []string{"/ping", "/salud", "/estado", "/estado/detalle", "/bitacora", "/metricas"} {
		manejador.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, ruta, nil))
	}
	if salida.Len() != 0 {
		t.Fatalf("los sondeos no debian trazarse: %q", salida.String())
	}

	registrador2, salida2 := registradorDePrueba()
	conPings := ConRegistroPeticiones(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"replica_id":"A"}`))
	}), OpcionesRegistro{Activo: true, IncluirVigilancia: true, Registrador: registrador2})
	conPings.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ping", nil))
	if !strings.Contains(salida2.String(), "/ping") {
		t.Fatalf("con IncluirVigilancia el sondeo debe trazarse: %q", salida2.String())
	}
}

func TestRegistroApagadoNoEscribe(t *testing.T) {
	registrador, salida := registradorDePrueba()
	manejador := ConRegistroPeticiones(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}), OpcionesRegistro{Activo: false, Registrador: registrador})

	manejador.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/saldo/1234", nil))
	if salida.Len() != 0 {
		t.Fatalf("con LOG_PETICIONES=false no debe escribirse nada: %q", salida.String())
	}
}

// El gancho de caos responde y luego termina el proceso: el envoltorio del
// registro debe conservar la capacidad de vaciar el bufer (http.Flusher).
func TestRegistroConservaElFlusher(t *testing.T) {
	registrador, _ := registradorDePrueba()
	api := NuevaReplica("A", 15000, 0, 0)
	api.Registro = OpcionesRegistro{Activo: true, Registrador: registrador}
	terminado := make(chan int, 1)
	api.Salir = func(codigo int) { terminado <- codigo }

	grabadora := httptest.NewRecorder()
	api.Rutas().ServeHTTP(grabadora, httptest.NewRequest(http.MethodPost, "/chaos/crash", nil))

	if grabadora.Code != http.StatusOK {
		t.Fatalf("codigo %d", grabadora.Code)
	}
	select {
	case <-terminado:
	case <-time.After(time.Second):
		t.Fatal("la replica no invoco la terminacion del proceso")
	}
}

// El dispatcher traza la replica ganadora de cada carrera de redundancia.
func TestDispatcherTrazaLaReplicaGanadora(t *testing.T) {
	registrador, salida := registradorDePrueba()
	replicas := []domain.Replica{{ID: "A", URLBase: "http://a:8080"}, {ID: "B", URLBase: "http://b:8080"}}
	registro := usecase.NuevoRegistroReplicas(replicas, domain.EstadoViva, 2, 2)
	consulta := usecase.NuevoConsultarSaldo(registro, pasarelaDoble{}, time.Second, usecase.RelojSistema{})
	api := NuevoDispatcher(consulta, registro, nil, ConfigExpuesta{})
	api.Registro = OpcionesRegistro{Activo: true, Registrador: registrador}

	api.Rutas().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/saldo/1234", nil))

	linea := salida.String()
	if !strings.Contains(linea, "replica=A") && !strings.Contains(linea, "replica=B") {
		t.Fatalf("la traza debe indicar que replica gano: %q", linea)
	}
}

// Las replicas que pierden la carrera de la redundancia no deben aparecer como
// respuestas 200: el dispatcher cancelo esas peticiones.
func TestRegistroMarcaLasPeticionesCanceladas(t *testing.T) {
	registrador, salida := registradorDePrueba()
	manejador := ConRegistroPeticiones(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // el dispatcher ya tiene ganador y aborta
	}), OpcionesRegistro{Activo: true, Registrador: registrador})

	ctx, cancelar := context.WithCancel(context.Background())
	peticion := httptest.NewRequest(http.MethodGet, "/saldo/1234", nil).WithContext(ctx)
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancelar()
	}()
	manejador.ServeHTTP(httptest.NewRecorder(), peticion)

	linea := salida.String()
	if !strings.Contains(linea, "cancelada") || !strings.Contains(linea, "redundancia") {
		t.Fatalf("la traza debe marcar la peticion como cancelada: %q", linea)
	}
	if strings.Contains(linea, "-> 200") {
		t.Fatalf("una peticion cancelada no puede registrarse como 200: %q", linea)
	}
}
