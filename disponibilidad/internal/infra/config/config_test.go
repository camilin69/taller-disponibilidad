package config

import (
	"testing"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
)

func TestParsearReplicasFormatos(t *testing.T) {
	replicas, err := ParsearReplicas(" A=http://replica-a:8080, B|http://replica-b:8080 ,replica-c:8080 ")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(replicas) != 3 {
		t.Fatalf("se esperaban 3 replicas, se obtuvieron %d", len(replicas))
	}
	if replicas[0].ID != "A" || replicas[0].URLBase != "http://replica-a:8080" {
		t.Fatalf("replica 0 inesperada: %+v", replicas[0])
	}
	if replicas[1].ID != "B" {
		t.Fatalf("replica 1 inesperada: %+v", replicas[1])
	}
	// Sin identificador explicito se deduce del host: replica-c -> C.
	if replicas[2].ID != "C" || replicas[2].URLBase != "http://replica-c:8080" {
		t.Fatalf("replica 2 inesperada: %+v", replicas[2])
	}
}

func TestParsearReplicasErrores(t *testing.T) {
	if _, err := ParsearReplicas("  "); err == nil {
		t.Fatal("REPLICAS vacia debe fallar")
	}
	if _, err := ParsearReplicas("A=http://x:1,A=http://y:2"); err == nil {
		t.Fatal("identificadores duplicados deben fallar")
	}
}

// Agregar una cuarta replica es solo cambiar la variable de entorno (R2).
func TestAgregarReplicaSoloRequiereConfiguracion(t *testing.T) {
	base := "A=http://replica-a:8080,B=http://replica-b:8080,C=http://replica-c:8080"
	tres, _ := ParsearReplicas(base)
	cuatro, err := ParsearReplicas(base + ",D=http://replica-d:8080")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(tres) != 3 || len(cuatro) != 4 || cuatro[3].ID != "D" {
		t.Fatalf("la lista de replicas no escalo: %v / %v", tres, cuatro)
	}
}

func TestCargarDispatcherDesdeEntorno(t *testing.T) {
	t.Setenv("PUERTO", "9090")
	t.Setenv("REPLICAS", "A=http://a:1,B=http://b:2")
	t.Setenv("MONITOR_T_MS", "500")
	t.Setenv("MONITOR_TIMEOUT_MS", "120")
	t.Setenv("MONITOR_K", "3")
	t.Setenv("MONITOR_M", "4")
	t.Setenv("ESTADO_INICIAL", "CAIDA")
	t.Setenv("BITACORA_ARCHIVO", "/tmp/monitor.log")

	cfg, err := CargarDispatcher()
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if cfg.Puerto != 9090 || cfg.K != 3 || cfg.M != 4 {
		t.Fatalf("configuracion inesperada: %+v", cfg)
	}
	if cfg.IntervaloSondeo != 500*time.Millisecond || cfg.TimeoutSondeo != 120*time.Millisecond {
		t.Fatalf("tiempos inesperados: %+v", cfg)
	}
	if cfg.EstadoInicial != domain.EstadoCaida {
		t.Fatalf("estado inicial inesperado: %s", cfg.EstadoInicial)
	}
	if cfg.DeteccionEsperada() != 1500*time.Millisecond {
		t.Fatalf("deteccion esperada = %v, se esperaba 1.5s", cfg.DeteccionEsperada())
	}
}

func TestCargarDispatcherValidaciones(t *testing.T) {
	t.Setenv("REPLICAS", "A=http://a:1")
	if _, err := CargarDispatcher(); err == nil {
		t.Fatal("con una sola replica debe fallar (minimo 2)")
	}

	t.Setenv("REPLICAS", "A=http://a:1,B=http://b:2")
	t.Setenv("MONITOR_T_MS", "200")
	t.Setenv("MONITOR_TIMEOUT_MS", "500") // t > T
	if _, err := CargarDispatcher(); err == nil {
		t.Fatal("t debe ser menor que T")
	}
}

func TestCargarReplicaYCliente(t *testing.T) {
	t.Setenv("REPLICA_ID", "b")
	t.Setenv("PUERTO", "8081")
	t.Setenv("LATENCIA_MIN_MS", "30")
	t.Setenv("LATENCIA_MAX_MS", "10") // menor que el minimo: se normaliza
	rep, err := CargarReplica()
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if rep.ID != "B" || rep.Puerto != 8081 || rep.LatenciaMax != rep.LatenciaMin {
		t.Fatalf("configuracion de replica inesperada: %+v", rep)
	}

	t.Setenv("TASA_RPS", "20")
	t.Setenv("DURACION_S", "40")
	cli, err := CargarCliente()
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if cli.TasaRPS != 20 || cli.Duracion != 40*time.Second {
		t.Fatalf("configuracion de cliente inesperada: %+v", cli)
	}
}
