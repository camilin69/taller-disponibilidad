package calculo_test

import (
	"context"
	"testing"
	"time"

	"github.com/camilin69/taller-desempeno/internal/adapter/calculo"
	"github.com/camilin69/taller-desempeno/internal/domain"
)

// Restriccion 1 del enunciado: el calculo debe tener una duracion fija y
// comparable entre corridas, o los experimentos no se pueden comparar.
func TestCalcularTardaElCostoConfigurado(t *testing.T) {
	const costo = 120 * time.Millisecond
	c := calculo.NuevaViajes(costo)

	for i := 0; i < 3; i++ {
		inicio := time.Now()
		if _, err := c.Calcular(context.Background(), domain.ClaveResumen{IDTarjeta: "1234", Viajes: 40}); err != nil {
			t.Fatalf("el calculo fallo: %v", err)
		}
		transcurrido := time.Since(inicio)

		if transcurrido < costo {
			t.Fatalf("el calculo tardo %v, menos que el costo configurado %v", transcurrido, costo)
		}
		// Margen generoso: en un equipo cargado el planificador puede demorar.
		if transcurrido > costo+100*time.Millisecond {
			t.Fatalf("el calculo tardo %v, muy por encima del costo %v", transcurrido, costo)
		}
	}
}

func TestCalcularDevuelveElResumenDelDominio(t *testing.T) {
	c := calculo.NuevaViajes(time.Millisecond)
	clave := domain.ClaveResumen{IDTarjeta: "1234", Viajes: 40}

	resumen, err := c.Calcular(context.Background(), clave)
	if err != nil {
		t.Fatalf("el calculo fallo: %v", err)
	}
	if resumen.IDTarjeta != "1234" {
		t.Fatalf("id_tarjeta=%q, se esperaba \"1234\"", resumen.IDTarjeta)
	}
	if resumen.TotalViajes != 40 {
		t.Fatalf("total_viajes=%d, se esperaba 40", resumen.TotalViajes)
	}
	if esperado := domain.SumarViajes("1234", 40); resumen.GastoTotal != esperado {
		t.Fatalf("gasto_total=%d, se esperaba %d", resumen.GastoTotal, esperado)
	}
	if resumen.DesdeCache {
		t.Fatal("un calculo recien hecho no viene del cache")
	}
}

// Si quien pidio el resumen se rinde, el calculo debe abortar y liberar al
// trabajador en vez de terminar un trabajo que nadie va a leer.
func TestCalcularRespetaElContexto(t *testing.T) {
	c := calculo.NuevaViajes(2 * time.Second)

	ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancelar()

	inicio := time.Now()
	_, err := c.Calcular(ctx, domain.ClaveResumen{IDTarjeta: "1234", Viajes: 40})
	transcurrido := time.Since(inicio)

	if err == nil {
		t.Fatal("se esperaba un error al vencerse el contexto")
	}
	if transcurrido > 500*time.Millisecond {
		t.Fatalf("el calculo tardo %v en abortar; deberia cortar al vencerse el contexto", transcurrido)
	}
}
