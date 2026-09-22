package cache_test

import (
	"sync"
	"testing"

	"github.com/camilin69/taller-desempeno/internal/adapter/cache"
	"github.com/camilin69/taller-desempeno/internal/domain"
)

func clave(id string, viajes int) domain.ClaveResumen {
	return domain.ClaveResumen{IDTarjeta: id, Viajes: viajes}
}

func TestMemoriaGuardaYDevuelve(t *testing.T) {
	m := cache.NuevaMemoria()
	k := clave("1234", 40)

	if _, ok := m.Obtener(k); ok {
		t.Fatal("un cache vacio no deberia acertar")
	}

	guardado := domain.Resumen{IDTarjeta: "1234", TotalViajes: 40, GastoTotal: 152000}
	m.Guardar(k, guardado)

	obtenido, ok := m.Obtener(k)
	if !ok {
		t.Fatal("se esperaba un acierto tras guardar")
	}
	if obtenido != guardado {
		t.Fatalf("el cache devolvio %+v, se esperaba %+v", obtenido, guardado)
	}
	if m.Entradas() != 1 {
		t.Fatalf("entradas=%d, se esperaba 1", m.Entradas())
	}
}

func TestMemoriaSeparaClavesDistintas(t *testing.T) {
	m := cache.NuevaMemoria()
	m.Guardar(clave("1234", 40), domain.Resumen{GastoTotal: 100})
	m.Guardar(clave("1234", 60), domain.Resumen{GastoTotal: 200})
	m.Guardar(clave("9999", 40), domain.Resumen{GastoTotal: 300})

	if m.Entradas() != 3 {
		t.Fatalf("entradas=%d, se esperaban 3 claves distintas", m.Entradas())
	}
	if r, _ := m.Obtener(clave("1234", 60)); r.GastoTotal != 200 {
		t.Fatalf("gasto=%d, se esperaba 200", r.GastoTotal)
	}
}

// El cache lo comparten los W trabajadores, asi que tiene que aguantar
// lecturas y escrituras simultaneas. Esta prueba tiene sentido con -race.
func TestMemoriaEsSeguraEnConcurrencia(t *testing.T) {
	m := cache.NuevaMemoria()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			m.Guardar(clave("1234", n%10), domain.Resumen{GastoTotal: int64(n)})
		}(i)
		go func(n int) {
			defer wg.Done()
			_, _ = m.Obtener(clave("1234", n%10))
			_ = m.Entradas()
		}(i)
	}
	wg.Wait()

	if m.Entradas() != 10 {
		t.Fatalf("entradas=%d, se esperaban 10", m.Entradas())
	}
}

func TestDesactivadaNuncaAcierta(t *testing.T) {
	var d cache.Desactivada
	k := clave("1234", 40)

	d.Guardar(k, domain.Resumen{GastoTotal: 999})
	if _, ok := d.Obtener(k); ok {
		t.Fatal("el cache desactivado no deberia acertar nunca")
	}
	if d.Entradas() != 0 {
		t.Fatalf("entradas=%d, se esperaba 0", d.Entradas())
	}
	if d.Activa() {
		t.Fatal("Activa() deberia ser false")
	}
}
