package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/camilin69/taller-desempeno/internal/domain"
	"github.com/camilin69/taller-desempeno/internal/usecase"
)

// armar construye el caso de uso con el cache indicado y el pool ya arrancado.
func armar(t *testing.T, cache usecase.Cache, trabajadores int, duracion time.Duration) (*usecase.ObtenerResumen, *calculadoraDoble) {
	t.Helper()
	calc := nuevaCalculadoraDoble(duracion)
	pool := usecase.NuevoPool(calc, usecase.ConfigPool{Trabajadores: trabajadores, CapacidadCola: 64})

	ctx, cancelar := context.WithCancel(context.Background())
	t.Cleanup(cancelar)
	pool.Iniciar(ctx)

	return usecase.NuevoObtenerResumen(cache, pool), calc
}

// R2: la primera consulta calcula, la segunda debe venir del cache sin volver
// a ejecutar el calculo.
func TestSegundaConsultaVieneDelCache(t *testing.T) {
	uc, calc := armar(t, nuevoCacheDoble(), 4, 10*time.Millisecond)
	ctx := context.Background()
	k := clave("1234")

	primera, traza, err := uc.Ejecutar(ctx, k)
	if err != nil {
		t.Fatalf("primera consulta fallo: %v", err)
	}
	if traza.DesdeCache {
		t.Fatal("la primera consulta no puede venir del cache")
	}
	if primera.DesdeCache {
		t.Fatal("desde_cache deberia ser false en la primera consulta")
	}

	segunda, traza, err := uc.Ejecutar(ctx, k)
	if err != nil {
		t.Fatalf("segunda consulta fallo: %v", err)
	}
	if !traza.DesdeCache || !segunda.DesdeCache {
		t.Fatal("la segunda consulta deberia venir del cache")
	}
	if segunda.GastoTotal != primera.GastoTotal {
		t.Fatalf("el cache devolvio otro gasto: %d != %d", segunda.GastoTotal, primera.GastoTotal)
	}
	if veces := calc.vecesCalculada(k); veces != 1 {
		t.Fatalf("el calculo deberia ejecutarse una sola vez, se ejecuto %d", veces)
	}
}

// Un acierto de cache no debe pasar por el pool: si lo hiciera, haria fila
// detras de los calculos y dejaria de ser instantaneo.
func TestAciertoDeCacheNoOcupaTrabajadores(t *testing.T) {
	uc, _ := armar(t, nuevoCacheDoble(), 1, 40*time.Millisecond)
	ctx := context.Background()
	k := clave("1234")

	if _, _, err := uc.Ejecutar(ctx, k); err != nil {
		t.Fatalf("primera consulta fallo: %v", err)
	}
	encoladasAntes := uc.EstadoPool().Encoladas

	for i := 0; i < 5; i++ {
		if _, traza, _ := uc.Ejecutar(ctx, k); !traza.DesdeCache {
			t.Fatal("se esperaba un acierto de cache")
		}
	}

	if encoladasDespues := uc.EstadoPool().Encoladas; encoladasDespues != encoladasAntes {
		t.Fatalf("los aciertos de cache no deberian encolarse: %d -> %d",
			encoladasAntes, encoladasDespues)
	}
}

// La clave incluye el numero de viajes: pedir otros viajes es otro resumen.
func TestElCacheDistingueElNumeroDeViajes(t *testing.T) {
	uc, calc := armar(t, nuevoCacheDoble(), 4, 5*time.Millisecond)
	ctx := context.Background()

	cuarenta := domain.ClaveResumen{IDTarjeta: "1234", Viajes: 40}
	sesenta := domain.ClaveResumen{IDTarjeta: "1234", Viajes: 60}

	a, _, _ := uc.Ejecutar(ctx, cuarenta)
	b, traza, _ := uc.Ejecutar(ctx, sesenta)

	if traza.DesdeCache {
		t.Fatal("pedir otro numero de viajes no puede servirse del cache de 40")
	}
	if a.GastoTotal == b.GastoTotal {
		t.Fatal("40 y 60 viajes no deberian dar el mismo gasto")
	}
	if calc.vecesCalculada(sesenta) != 1 {
		t.Fatal("el resumen de 60 viajes deberia calcularse")
	}
}

// E0 y E1 corren con el cache apagado: toda solicitud ejecuta el calculo.
func TestConCacheApagadoSiempreSeCalcula(t *testing.T) {
	uc, calc := armar(t, cacheApagado{}, 4, 5*time.Millisecond)
	ctx := context.Background()
	k := clave("1234")

	for i := 0; i < 4; i++ {
		_, traza, err := uc.Ejecutar(ctx, k)
		if err != nil {
			t.Fatalf("consulta %d fallo: %v", i, err)
		}
		if traza.DesdeCache {
			t.Fatal("con el cache apagado ninguna respuesta puede venir del cache")
		}
	}
	if veces := calc.vecesCalculada(k); veces != 4 {
		t.Fatalf("se esperaban 4 calculos, hubo %d", veces)
	}
	if estado := uc.EstadoCache(); estado.Activa {
		t.Fatal("EstadoCache deberia reportar el cache apagado")
	}
}

func TestEstadoCacheCuentaAciertosYFallos(t *testing.T) {
	uc, _ := armar(t, nuevoCacheDoble(), 4, time.Millisecond)
	ctx := context.Background()
	k := clave("1234")

	_, _, _ = uc.Ejecutar(ctx, k) // fallo (calcula)
	_, _, _ = uc.Ejecutar(ctx, k) // acierto
	_, _, _ = uc.Ejecutar(ctx, k) // acierto

	estado := uc.EstadoCache()
	if estado.Fallos != 1 {
		t.Fatalf("fallos=%d, se esperaba 1", estado.Fallos)
	}
	if estado.Aciertos != 2 {
		t.Fatalf("aciertos=%d, se esperaba 2", estado.Aciertos)
	}
	if estado.Entradas != 1 {
		t.Fatalf("entradas=%d, se esperaba 1", estado.Entradas)
	}
	if estado.Tasa < 66 || estado.Tasa > 67 {
		t.Fatalf("tasa=%.2f, se esperaba ~66.67", estado.Tasa)
	}
}

// Un calculo fallido no debe quedar guardado: el cache solo almacena
// resultados validos.
func TestUnCalculoFallidoNoSeGuarda(t *testing.T) {
	cache := nuevoCacheDoble()
	uc, _ := armar(t, cache, 1, 200*time.Millisecond)
	k := clave("1234")

	ctx, cancelar := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelar()

	if _, _, err := uc.Ejecutar(ctx, k); err == nil {
		t.Fatal("se esperaba que la consulta venciera su tiempo limite")
	}
	if cache.Entradas() != 0 {
		t.Fatal("un calculo que no termino no deberia dejar entrada en el cache")
	}
}
