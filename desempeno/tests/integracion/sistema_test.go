// Package integracion arma el sistema completo (el mismo composition root que
// usa el binario) y lo somete a una rafaga real por HTTP. Es la prueba de que
// las dos tacticas producen el efecto que el taller espera medir.
package integracion_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/camilin69/taller-desempeno/internal/infra/config"
	"github.com/camilin69/taller-desempeno/internal/infra/ensamblaje"
)

// costoPrueba es corto a proposito: conserva la proporcion entre carga y
// capacidad sin que la suite tarde medio minuto.
const costoPrueba = 40 * time.Millisecond

// levantar arma el sistema y lo expone en un servidor HTTP de prueba.
func levantar(t *testing.T, trabajadores int, cacheActiva bool) *httptest.Server {
	t.Helper()

	sistema, err := ensamblaje.ArmarServidor(config.Servidor{
		Puerto:           8080,
		Trabajadores:     trabajadores,
		CapacidadCola:    256,
		CacheActiva:      cacheActiva,
		CostoCalculo:     costoPrueba,
		ViajesPorDefecto: 40,
		LogPeticiones:    false,
	})
	if err != nil {
		t.Fatalf("no se pudo armar el sistema: %v", err)
	}

	ctx, cancelar := context.WithCancel(context.Background())
	sistema.IniciarTrabajadores(ctx)

	srv := httptest.NewServer(sistema.API)
	t.Cleanup(func() {
		srv.Close()
		cancelar()
	})
	return srv
}

// resultadoRafaga son los numeros que deja una rafaga de prueba.
type resultadoRafaga struct {
	exitosas   int
	desdeCache int
	duracion   time.Duration
}

// dispararRafaga lanza n solicitudes simultaneas rotando las tarjetas dadas.
func dispararRafaga(t *testing.T, urlBase string, n, numTarjetas int) resultadoRafaga {
	t.Helper()

	cliente := &http.Client{Timeout: 5 * time.Second}
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		res resultadoRafaga
	)

	inicio := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			tarjeta := 1000 + n%numTarjetas
			respuesta, err := cliente.Get(fmt.Sprintf("%s/resumen/%d?viajes=40", urlBase, tarjeta))
			if err != nil {
				return
			}
			defer func() {
				_, _ = io.Copy(io.Discard, respuesta.Body)
				_ = respuesta.Body.Close()
			}()
			if respuesta.StatusCode != http.StatusOK {
				return
			}

			var cuerpo struct {
				GastoTotal int64 `json:"gasto_total"`
				DesdeCache bool  `json:"desde_cache"`
			}
			if err := json.NewDecoder(respuesta.Body).Decode(&cuerpo); err != nil || cuerpo.GastoTotal <= 0 {
				return
			}

			mu.Lock()
			res.exitosas++
			if cuerpo.DesdeCache {
				res.desdeCache++
			}
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	res.duracion = time.Since(inicio)
	return res
}

// E0 vs E1: mas trabajadores deben atender la misma rafaga en menos tiempo.
// Es la comparacion de throughput que pide la pregunta de analisis 1.
func TestConcurrenciaMejoraElThroughput(t *testing.T) {
	const solicitudes = 24

	e0 := levantar(t, 1, false)
	lineaBase := dispararRafaga(t, e0.URL, solicitudes, 10)

	e1 := levantar(t, 8, false)
	conConcurrencia := dispararRafaga(t, e1.URL, solicitudes, 10)

	if lineaBase.exitosas != solicitudes || conConcurrencia.exitosas != solicitudes {
		t.Fatalf("se perdieron solicitudes: E0=%d, E1=%d de %d",
			lineaBase.exitosas, conConcurrencia.exitosas, solicitudes)
	}

	throughputE0 := float64(lineaBase.exitosas) / lineaBase.duracion.Seconds()
	throughputE1 := float64(conConcurrencia.exitosas) / conConcurrencia.duracion.Seconds()
	if throughputE1 <= throughputE0 {
		t.Fatalf("W=8 (%.1f req/s) deberia superar a W=1 (%.1f req/s)", throughputE1, throughputE0)
	}

	// Con W=1 la rafaga se serializa: 24 calculos de 40 ms son al menos ~960 ms.
	minimoSerial := time.Duration(solicitudes) * costoPrueba
	if lineaBase.duracion < minimoSerial {
		t.Fatalf("con W=1 la rafaga tardo %v; no pudo haberse serializado (minimo %v)",
			lineaBase.duracion, minimoSerial)
	}
}

// E2: con pocas tarjetas, la mayoria de las solicitudes debe encontrar el
// resultado ya calculado.
func TestElCacheSirveLaMayoriaDeLasRepeticiones(t *testing.T) {
	srv := levantar(t, 8, true)

	// Primera pasada: calienta el cache con las 10 tarjetas.
	dispararRafaga(t, srv.URL, 10, 10)

	// Segunda pasada: todo deberia venir del cache.
	segunda := dispararRafaga(t, srv.URL, 40, 10)
	if segunda.exitosas != 40 {
		t.Fatalf("exitosas=%d, se esperaban 40", segunda.exitosas)
	}
	if segunda.desdeCache != 40 {
		t.Fatalf("desde cache=%d de 40; con el cache caliente deberian serlo todas", segunda.desdeCache)
	}

	// Y debe ser mucho mas rapido que calcular: 40 calculos en serie de a 8
	// tomarian al menos 5 tandas de 40 ms.
	if segunda.duracion > 20*costoPrueba {
		t.Fatalf("la pasada desde cache tardo %v; deberia ser casi instantanea", segunda.duracion)
	}
}

// GET /config es la verificacion en vivo que pide el enunciado.
func TestConfigReflejaElExperimentoEnCurso(t *testing.T) {
	srv := levantar(t, 8, true)
	dispararRafaga(t, srv.URL, 10, 10)

	respuesta, err := http.Get(srv.URL + "/config")
	if err != nil {
		t.Fatalf("no se pudo consultar /config: %v", err)
	}
	defer func() { _ = respuesta.Body.Close() }()

	var cuerpo struct {
		Trabajadores  int  `json:"trabajadores"`
		CacheActiva   bool `json:"cache_activa"`
		CacheEntradas int  `json:"cache_entradas"`
		Completadas   int  `json:"solicitudes_completadas"`
	}
	if err := json.NewDecoder(respuesta.Body).Decode(&cuerpo); err != nil {
		t.Fatalf("cuerpo ilegible: %v", err)
	}
	if cuerpo.Trabajadores != 8 {
		t.Fatalf("trabajadores=%d, se esperaba 8", cuerpo.Trabajadores)
	}
	if !cuerpo.CacheActiva {
		t.Fatal("cache_activa deberia ser true")
	}
	if cuerpo.CacheEntradas != 10 {
		t.Fatalf("cache_entradas=%d, se esperaban 10 (una por tarjeta)", cuerpo.CacheEntradas)
	}
	if cuerpo.Completadas < 10 {
		t.Fatalf("solicitudes_completadas=%d, se esperaban al menos 10", cuerpo.Completadas)
	}
}

// Todas las replicas de una misma tarjeta deben dar el mismo gasto, venga del
// cache o del calculo: si no, el cache estaria devolviendo datos equivocados.
func TestElCacheNoAlteraElResultado(t *testing.T) {
	sinCache := levantar(t, 4, false)
	conCache := levantar(t, 4, true)

	leerGasto := func(urlBase string) int64 {
		t.Helper()
		respuesta, err := http.Get(urlBase + "/resumen/1234?viajes=40")
		if err != nil {
			t.Fatalf("consulta fallida: %v", err)
		}
		defer func() { _ = respuesta.Body.Close() }()
		var cuerpo struct {
			GastoTotal int64 `json:"gasto_total"`
		}
		if err := json.NewDecoder(respuesta.Body).Decode(&cuerpo); err != nil {
			t.Fatalf("cuerpo ilegible: %v", err)
		}
		return cuerpo.GastoTotal
	}

	calculado := leerGasto(sinCache.URL)
	leerGasto(conCache.URL) // calienta el cache
	servidoDelCache := leerGasto(conCache.URL)

	if calculado != servidoDelCache {
		t.Fatalf("el cache devolvio %d y el calculo %d", servidoDelCache, calculado)
	}
}
