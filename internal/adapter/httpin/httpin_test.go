package httpin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
	"github.com/camilin69/taller-disponibilidad/internal/usecase"
)

// pasarelaDoble simula el acceso HTTP a las replicas sin red.
type pasarelaDoble struct {
	err error
}

func (p pasarelaDoble) ConsultarSaldo(_ context.Context, r domain.Replica, idTarjeta string) (domain.Saldo, error) {
	if p.err != nil {
		return domain.Saldo{}, p.err
	}
	return domain.Saldo{ReplicaID: r.ID, IDTarjeta: idTarjeta, Valor: 15000}, nil
}

func dispatcherDePrueba(t *testing.T, pasarela usecase.PasarelaSaldo) (*Dispatcher, *usecase.RegistroReplicas) {
	t.Helper()
	replicas := []domain.Replica{
		{ID: "A", URLBase: "http://a:8080"},
		{ID: "B", URLBase: "http://b:8080"},
	}
	registro := usecase.NuevoRegistroReplicas(replicas, domain.EstadoViva, 2, 2)
	consulta := usecase.NuevoConsultarSaldo(registro, pasarela, time.Second, usecase.RelojSistema{})
	api := NuevoDispatcher(consulta, registro, nil, ConfigExpuesta{
		IntervaloSondeoMS: 1000, TimeoutSondeoMS: 300, K: 2, M: 2, DeteccionEsperadaMS: 2000,
	})
	return api, registro
}

// GET /estado debe cumplir el contrato {"A":"VIVA","B":"CAIDA"}.
func TestDispatcherEstadoDevuelveElContratoDelTaller(t *testing.T) {
	api, registro := dispatcherDePrueba(t, pasarelaDoble{})
	registro.RegistrarSondeo("B", false, 0, time.Now())
	registro.RegistrarSondeo("B", false, 0, time.Now()) // k=2 -> CAIDA

	grabadora := httptest.NewRecorder()
	api.Rutas().ServeHTTP(grabadora, httptest.NewRequest(http.MethodGet, "/estado", nil))

	if grabadora.Code != http.StatusOK {
		t.Fatalf("codigo %d", grabadora.Code)
	}
	var estados map[string]string
	if err := json.Unmarshal(grabadora.Body.Bytes(), &estados); err != nil {
		t.Fatalf("cuerpo ilegible: %v", err)
	}
	if estados["A"] != "VIVA" || estados["B"] != "CAIDA" {
		t.Fatalf("estados inesperados: %v", estados)
	}
}

func TestDispatcherSaldoResponde200ConReplicaGanadora(t *testing.T) {
	api, _ := dispatcherDePrueba(t, pasarelaDoble{})

	grabadora := httptest.NewRecorder()
	api.Rutas().ServeHTTP(grabadora, httptest.NewRequest(http.MethodGet, "/saldo/1234", nil))

	if grabadora.Code != http.StatusOK {
		t.Fatalf("codigo %d, se esperaba 200", grabadora.Code)
	}
	var cuerpo RespuestaSaldo
	if err := json.Unmarshal(grabadora.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("cuerpo ilegible: %v", err)
	}
	if cuerpo.ReplicaID == "" || cuerpo.Saldo != 15000 || cuerpo.IDTarjeta != "1234" {
		t.Fatalf("respuesta inesperada: %+v", cuerpo)
	}
	if grabadora.Header().Get("X-Replica-Id") != cuerpo.ReplicaID {
		t.Fatal("falta el encabezado X-Replica-Id")
	}
}

func TestDispatcherSaldoValidaLaTarjetaYElMetodo(t *testing.T) {
	api, _ := dispatcherDePrueba(t, pasarelaDoble{})

	grabadora := httptest.NewRecorder()
	api.Rutas().ServeHTTP(grabadora, httptest.NewRequest(http.MethodGet, "/saldo/", nil))
	if grabadora.Code != http.StatusBadRequest {
		t.Fatalf("id vacio: codigo %d, se esperaba 400", grabadora.Code)
	}

	grabadora = httptest.NewRecorder()
	api.Rutas().ServeHTTP(grabadora, httptest.NewRequest(http.MethodPost, "/saldo/1234", nil))
	if grabadora.Code != http.StatusMethodNotAllowed {
		t.Fatalf("metodo POST: codigo %d, se esperaba 405", grabadora.Code)
	}
}

// Sin replicas VIVA el dispatcher responde 503, no se cuelga ni devuelve 200.
func TestDispatcherSaldoResponde503SinReplicasVivas(t *testing.T) {
	api, registro := dispatcherDePrueba(t, pasarelaDoble{})
	for _, id := range []string{"A", "B"} {
		registro.RegistrarSondeo(id, false, 0, time.Now())
		registro.RegistrarSondeo(id, false, 0, time.Now())
	}

	grabadora := httptest.NewRecorder()
	api.Rutas().ServeHTTP(grabadora, httptest.NewRequest(http.MethodGet, "/saldo/1234", nil))

	if grabadora.Code != http.StatusServiceUnavailable {
		t.Fatalf("codigo %d, se esperaba 503", grabadora.Code)
	}
	var err ErrorHTTP
	_ = json.Unmarshal(grabadora.Body.Bytes(), &err)
	if err.Error == "" {
		t.Fatal("el error deberia venir con formato uniforme")
	}
}

// El panel React se sirve desde otro origen: la API debe permitir CORS.
func TestDispatcherPermiteCORS(t *testing.T) {
	api, _ := dispatcherDePrueba(t, pasarelaDoble{})

	grabadora := httptest.NewRecorder()
	api.Rutas().ServeHTTP(grabadora, httptest.NewRequest(http.MethodOptions, "/estado", nil))

	if grabadora.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("falta el encabezado CORS")
	}
	if grabadora.Code != http.StatusNoContent {
		t.Fatalf("preflight respondio %d", grabadora.Code)
	}
}

func TestDispatcherDetalleIncluyeConfiguracionYMetricas(t *testing.T) {
	api, _ := dispatcherDePrueba(t, pasarelaDoble{})
	api.Rutas().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/saldo/1234", nil))

	grabadora := httptest.NewRecorder()
	api.Rutas().ServeHTTP(grabadora, httptest.NewRequest(http.MethodGet, "/estado/detalle", nil))

	var detalle RespuestaEstadoDetalle
	if err := json.Unmarshal(grabadora.Body.Bytes(), &detalle); err != nil {
		t.Fatalf("cuerpo ilegible: %v", err)
	}
	if detalle.Config.K != 2 || detalle.Config.IntervaloSondeoMS != 1000 {
		t.Fatalf("configuracion expuesta inesperada: %+v", detalle.Config)
	}
	if len(detalle.Replicas) != 2 || detalle.Metricas.Total != 1 {
		t.Fatalf("detalle inesperado: %+v", detalle)
	}
}

// --- adaptador de la replica ---

func TestReplicaPingDevuelveElEco(t *testing.T) {
	api := NuevaReplica("A", 15000, 0, 0)

	grabadora := httptest.NewRecorder()
	api.Rutas().ServeHTTP(grabadora, httptest.NewRequest(http.MethodGet, "/ping", nil))

	if grabadora.Code != http.StatusOK {
		t.Fatalf("codigo %d", grabadora.Code)
	}
	var cuerpo map[string]string
	_ = json.Unmarshal(grabadora.Body.Bytes(), &cuerpo)
	if cuerpo["replica_id"] != "A" {
		t.Fatalf("eco inesperado: %v", cuerpo)
	}
}

// Todas las replicas devuelven el MISMO saldo para la misma tarjeta.
func TestReplicaSaldoEsDeterministaEntreReplicas(t *testing.T) {
	a := NuevaReplica("A", 15000, 0, 0)
	b := NuevaReplica("B", 15000, 0, 0)

	leer := func(api *Replica) domain.Saldo {
		grabadora := httptest.NewRecorder()
		api.Rutas().ServeHTTP(grabadora, httptest.NewRequest(http.MethodGet, "/saldo/1234", nil))
		if grabadora.Code != http.StatusOK {
			t.Fatalf("codigo %d", grabadora.Code)
		}
		var saldo domain.Saldo
		_ = json.Unmarshal(grabadora.Body.Bytes(), &saldo)
		return saldo
	}

	saldoA, saldoB := leer(a), leer(b)
	if saldoA.Valor != saldoB.Valor {
		t.Fatalf("las replicas devolvieron saldos distintos: %d vs %d", saldoA.Valor, saldoB.Valor)
	}
	if saldoA.ReplicaID != "A" || saldoB.ReplicaID != "B" {
		t.Fatalf("cada replica debe identificarse: %s / %s", saldoA.ReplicaID, saldoB.ReplicaID)
	}
	if !saldoA.EsValida() {
		t.Fatalf("saldo invalido: %+v", saldoA)
	}
}

func TestReplicaCrashSoloAceptaPost(t *testing.T) {
	api := NuevaReplica("A", 15000, 0, 0)
	api.Salir = func(int) {}

	grabadora := httptest.NewRecorder()
	api.Rutas().ServeHTTP(grabadora, httptest.NewRequest(http.MethodGet, "/chaos/crash", nil))

	if grabadora.Code != http.StatusMethodNotAllowed {
		t.Fatalf("codigo %d, se esperaba 405", grabadora.Code)
	}
}
