package config_test

import (
	"testing"
	"time"

	"github.com/camilin69/taller-desempeno/internal/infra/config"
)

func TestCargarServidorUsaLosDefectosDeE0(t *testing.T) {
	cfg, err := config.CargarServidor()
	if err != nil {
		t.Fatalf("configuracion por defecto invalida: %v", err)
	}
	// Los defectos son deliberadamente los de la linea base: si alguien levanta
	// el servidor sin variables, obtiene E0 (W=1, sin cache).
	if cfg.Trabajadores != 1 {
		t.Fatalf("TRABAJADORES por defecto=%d, se esperaba 1", cfg.Trabajadores)
	}
	if cfg.CacheActiva {
		t.Fatal("el cache deberia venir apagado por defecto")
	}
	if cfg.CostoCalculo != 200*time.Millisecond {
		t.Fatalf("COSTO_CALCULO_MS por defecto=%v, se esperaba 200ms", cfg.CostoCalculo)
	}
}

func TestCargarServidorLeeElEntorno(t *testing.T) {
	t.Setenv("TRABAJADORES", "8")
	t.Setenv("COLA_MAX", "512")
	t.Setenv("CACHE_ACTIVA", "true")
	t.Setenv("COSTO_CALCULO_MS", "150")

	cfg, err := config.CargarServidor()
	if err != nil {
		t.Fatalf("configuracion invalida: %v", err)
	}
	if cfg.Trabajadores != 8 {
		t.Fatalf("trabajadores=%d, se esperaba 8", cfg.Trabajadores)
	}
	if cfg.CapacidadCola != 512 {
		t.Fatalf("cola=%d, se esperaba 512", cfg.CapacidadCola)
	}
	if !cfg.CacheActiva {
		t.Fatal("CACHE_ACTIVA=true deberia encender el cache")
	}
	if cfg.CostoCalculo != 150*time.Millisecond {
		t.Fatalf("costo=%v, se esperaba 150ms", cfg.CostoCalculo)
	}
}

func TestServidorValidar(t *testing.T) {
	base := config.Servidor{Puerto: 8080, Trabajadores: 1, CapacidadCola: 1,
		CostoCalculo: time.Millisecond, ViajesPorDefecto: 1}
	if err := base.Validar(); err != nil {
		t.Fatalf("configuracion minima rechazada: %v", err)
	}

	invalidas := map[string]config.Servidor{
		"sin trabajadores": {Puerto: 8080, Trabajadores: 0, CapacidadCola: 1, CostoCalculo: time.Millisecond, ViajesPorDefecto: 1},
		"sin cola":         {Puerto: 8080, Trabajadores: 1, CapacidadCola: 0, CostoCalculo: time.Millisecond, ViajesPorDefecto: 1},
		"sin costo":        {Puerto: 8080, Trabajadores: 1, CapacidadCola: 1, CostoCalculo: 0, ViajesPorDefecto: 1},
		"puerto invalido":  {Puerto: 0, Trabajadores: 1, CapacidadCola: 1, CostoCalculo: time.Millisecond, ViajesPorDefecto: 1},
		"sin viajes":       {Puerto: 8080, Trabajadores: 1, CapacidadCola: 1, CostoCalculo: time.Millisecond, ViajesPorDefecto: 0},
	}
	for nombre, cfg := range invalidas {
		t.Run(nombre, func(t *testing.T) {
			if err := cfg.Validar(); err == nil {
				t.Fatal("se esperaba que la configuracion fuera rechazada")
			}
		})
	}
}

// La capacidad teorica explica por que E0 satura y E1 no: con W=1 y 200 ms el
// servidor solo da 5 req/s, muy por debajo de la tasa de la rafaga.
func TestCapacidadTeorica(t *testing.T) {
	e0 := config.Servidor{Trabajadores: 1, CostoCalculo: 200 * time.Millisecond}
	if capacidad := e0.CapacidadTeorica(); capacidad != 5 {
		t.Fatalf("capacidad con W=1=%.1f req/s, se esperaba 5", capacidad)
	}

	e1 := config.Servidor{Trabajadores: 8, CostoCalculo: 200 * time.Millisecond}
	if capacidad := e1.CapacidadTeorica(); capacidad != 40 {
		t.Fatalf("capacidad con W=8=%.1f req/s, se esperaba 40", capacidad)
	}
}

func TestClienteTarjetasRotanUnConjuntoPequeno(t *testing.T) {
	t.Setenv("NUM_TARJETAS", "10")
	t.Setenv("TARJETA_BASE", "1000")

	cfg, err := config.CargarCliente()
	if err != nil {
		t.Fatalf("configuracion invalida: %v", err)
	}
	tarjetas := cfg.Tarjetas()
	if len(tarjetas) != 10 {
		t.Fatalf("se generaron %d tarjetas, se esperaban 10", len(tarjetas))
	}
	if tarjetas[0] != "1000" || tarjetas[9] != "1009" {
		t.Fatalf("rango de tarjetas=%s..%s, se esperaba 1000..1009", tarjetas[0], tarjetas[9])
	}
}

// El enunciado pide al menos 100 solicitudes en 15 a 20 segundos.
func TestSolicitudesEsperadasCumpleElProtocolo(t *testing.T) {
	cfg, err := config.CargarCliente()
	if err != nil {
		t.Fatalf("configuracion invalida: %v", err)
	}
	if n := cfg.SolicitudesEsperadas(); n < 100 {
		t.Fatalf("la rafaga por defecto dispara %d solicitudes, el taller pide al menos 100", n)
	}
	if s := cfg.Duracion.Seconds(); s < 15 || s > 20 {
		t.Fatalf("la duracion por defecto es %.0f s, el taller pide entre 15 y 20", s)
	}
}

func TestCargarClienteRechazaValoresInvalidos(t *testing.T) {
	t.Run("tasa cero", func(t *testing.T) {
		t.Setenv("TASA_RPS", "0")
		if _, err := config.CargarCliente(); err == nil {
			t.Fatal("TASA_RPS=0 deberia rechazarse")
		}
	})
	t.Run("duracion cero", func(t *testing.T) {
		t.Setenv("DURACION_S", "0")
		if _, err := config.CargarCliente(); err == nil {
			t.Fatal("DURACION_S=0 deberia rechazarse")
		}
	})
	t.Run("sin tarjetas", func(t *testing.T) {
		t.Setenv("NUM_TARJETAS", "0")
		if _, err := config.CargarCliente(); err == nil {
			t.Fatal("NUM_TARJETAS=0 deberia rechazarse")
		}
	})
}
