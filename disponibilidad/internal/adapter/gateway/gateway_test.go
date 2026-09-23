package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
)

// servidorFalso levanta una replica simulada con el manejador dado.
func servidorFalso(t *testing.T, manejador http.HandlerFunc) (domain.Replica, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(manejador)
	t.Cleanup(srv.Close)
	return domain.Replica{ID: "A", URLBase: srv.URL}, srv
}

func TestSondeadorAceptaEcoValido(t *testing.T) {
	replica, _ := servidorFalso(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ping" {
			t.Errorf("ruta sondeada = %q, se esperaba /ping", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"replica_id":"A"}`))
	})

	if err := NuevoSondeadorHTTP(2).Sondear(context.Background(), replica); err != nil {
		t.Fatalf("un eco valido no debio fallar: %v", err)
	}
}

// Ping/ECHO: el eco debe identificar a la replica sondeada.
func TestSondeadorRechazaEcoDeOtraReplica(t *testing.T) {
	replica, _ := servidorFalso(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"replica_id":"Z"}`))
	})

	if err := NuevoSondeadorHTTP(2).Sondear(context.Background(), replica); err == nil {
		t.Fatal("un eco con identificador ajeno debe contar como fallo")
	}
}

func TestSondeadorRechazaCodigoDistintoDe200YCuerpoIlegible(t *testing.T) {
	replicaError, _ := servidorFalso(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if err := NuevoSondeadorHTTP(2).Sondear(context.Background(), replicaError); err == nil {
		t.Fatal("un 500 debe contar como fallo de sondeo")
	}

	replicaBasura, _ := servidorFalso(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("no soy json"))
	})
	if err := NuevoSondeadorHTTP(2).Sondear(context.Background(), replicaBasura); err == nil {
		t.Fatal("un cuerpo ilegible debe contar como fallo de sondeo")
	}
}

// El tiempo limite t llega por context: si vence, el sondeo falla.
func TestSondeadorRespetaElTiempoLimite(t *testing.T) {
	replica, _ := servidorFalso(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
			_, _ = w.Write([]byte(`{"replica_id":"A"}`))
		case <-r.Context().Done():
		}
	})

	ctx, cancelar := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelar()

	inicio := time.Now()
	if err := NuevoSondeadorHTTP(2).Sondear(ctx, replica); err == nil {
		t.Fatal("un eco que excede t debe fallar")
	}
	if transcurrido := time.Since(inicio); transcurrido > time.Second {
		t.Fatalf("el sondeo no respeto el tiempo limite: %v", transcurrido)
	}
}

func TestSondeadorFallaSiLaReplicaNoEscucha(t *testing.T) {
	_, srv := servidorFalso(t, func(w http.ResponseWriter, _ *http.Request) {})
	replica := domain.Replica{ID: "A", URLBase: srv.URL}
	srv.Close() // la replica "muere"

	if err := NuevoSondeadorHTTP(2).Sondear(context.Background(), replica); err == nil {
		t.Fatal("una replica muerta debe producir fallo de sondeo")
	}
}

func TestPasarelaDevuelveElSaldo(t *testing.T) {
	replica, _ := servidorFalso(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/saldo/1234" {
			t.Errorf("ruta consultada = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"replica_id":"A","saldo":15000}`))
	})

	saldo, err := NuevaPasarelaSaldoHTTP(8).ConsultarSaldo(context.Background(), replica, "1234")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if saldo.ReplicaID != "A" || saldo.Valor != 15000 || saldo.IDTarjeta != "1234" {
		t.Fatalf("saldo inesperado: %+v", saldo)
	}
}

func TestPasarelaRechazaRespuestasNoValidas(t *testing.T) {
	replica503, _ := servidorFalso(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	if _, err := NuevaPasarelaSaldoHTTP(8).ConsultarSaldo(context.Background(), replica503, "1234"); err == nil {
		t.Fatal("un 503 no puede tomarse como respuesta valida")
	}

	replicaBasura, _ := servidorFalso(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>error</html>"))
	})
	if _, err := NuevaPasarelaSaldoHTTP(8).ConsultarSaldo(context.Background(), replicaBasura, "1234"); err == nil {
		t.Fatal("un cuerpo ilegible no puede tomarse como respuesta valida")
	}
}
