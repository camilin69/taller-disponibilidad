package domain_test

import (
	"errors"
	"testing"

	"github.com/camilin69/taller-desempeno/internal/domain"
)

func TestValidarIDTarjeta(t *testing.T) {
	casos := []struct {
		nombre   string
		id       string
		esValido bool
	}{
		{"identificador normal", "1234", true},
		{"con espacios alrededor", "  1234  ", true},
		{"vacio", "", false},
		{"solo espacios", "   ", false},
		{"con separador de ruta", "12/34", false},
		{"con query", "1234?x=1", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := domain.ValidarIDTarjeta(c.id)
			if c.esValido && err != nil {
				t.Fatalf("se esperaba %q valido, se obtuvo %v", c.id, err)
			}
			if !c.esValido && err == nil {
				t.Fatalf("se esperaba que %q fuera rechazado", c.id)
			}
		})
	}
}

func TestValidarViajes(t *testing.T) {
	if err := domain.ValidarViajes(40); err != nil {
		t.Fatalf("40 viajes deberia ser valido: %v", err)
	}
	if err := domain.ValidarViajes(0); err == nil {
		t.Fatal("0 viajes deberia rechazarse")
	}
	if err := domain.ValidarViajes(-5); err == nil {
		t.Fatal("un numero negativo deberia rechazarse")
	}
	if err := domain.ValidarViajes(domain.MaxViajes + 1); err == nil {
		t.Fatal("superar MaxViajes deberia rechazarse")
	}
}

// El cache solo es correcto si el calculo es determinista: la respuesta
// guardada debe ser identica a la que produciria recalcular.
func TestSumarViajesEsDeterminista(t *testing.T) {
	primera := domain.SumarViajes("1234", 40)
	for i := 0; i < 5; i++ {
		if otra := domain.SumarViajes("1234", 40); otra != primera {
			t.Fatalf("el calculo no es determinista: %d != %d", otra, primera)
		}
	}
}

// Cachear solo por tarjeta seria un error: pedir mas viajes debe dar mas gasto.
func TestSumarViajesCreceConElNumeroDeViajes(t *testing.T) {
	cuarenta := domain.SumarViajes("1234", 40)
	sesenta := domain.SumarViajes("1234", 60)
	if sesenta <= cuarenta {
		t.Fatalf("60 viajes (%d) deberia gastar mas que 40 (%d)", sesenta, cuarenta)
	}
}

func TestSumarViajesDistingueTarjetas(t *testing.T) {
	if domain.SumarViajes("1000", 40) == domain.SumarViajes("1001", 40) {
		t.Fatal("dos tarjetas distintas no deberian dar el mismo gasto")
	}
}

func TestSumarViajesEsLaSumaDeSusTarifas(t *testing.T) {
	var esperado int64
	for n := 1; n <= 12; n++ {
		esperado += domain.TarifaViaje("1234", n)
	}
	if obtenido := domain.SumarViajes("1234", 12); obtenido != esperado {
		t.Fatalf("suma=%d, se esperaba %d", obtenido, esperado)
	}
}

func TestClaveResumenString(t *testing.T) {
	clave := domain.ClaveResumen{IDTarjeta: "1234", Viajes: 40}
	if clave.String() != "1234/40" {
		t.Fatalf("String()=%q, se esperaba \"1234/40\"", clave.String())
	}
}

func TestErrColaLlenaEsComparable(t *testing.T) {
	if !errors.Is(domain.ErrColaLlena, domain.ErrColaLlena) {
		t.Fatal("ErrColaLlena deberia ser identificable con errors.Is")
	}
}
