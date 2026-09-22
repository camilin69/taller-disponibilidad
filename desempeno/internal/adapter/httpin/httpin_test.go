package httpin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/camilin69/taller-desempeno/internal/adapter/cache"
	"github.com/camilin69/taller-desempeno/internal/adapter/calculo"
	"github.com/camilin69/taller-desempeno/internal/adapter/httpin"
	"github.com/camilin69/taller-desempeno/internal/usecase"
)

// armarAPI levanta la API con un calculo barato para que las pruebas sean
// rapidas; el comportamiento que se verifica no depende del costo.
func armarAPI(t *testing.T, trabajadores, cola int, cacheActiva bool) http.Handler {
	t.Helper()

	var almacen usecase.Cache = cache.Desactivada{}
	if cacheActiva {
		almacen = cache.NuevaMemoria()
	}
	pool := usecase.NuevoPool(calculo.NuevaViajes(5*time.Millisecond),
		usecase.ConfigPool{Trabajadores: trabajadores, CapacidadCola: cola})

	ctx, cancelar := context.WithCancel(context.Background())
	t.Cleanup(cancelar)
	pool.Iniciar(ctx)

	api := httpin.NuevaAPI(usecase.NuevoObtenerResumen(almacen, pool),
		httpin.ConfigExpuesta{CostoCalculoMS: 5, ViajesPorDefecto: 40})
	return api.Rutas()
}

// pedirResumen ejecuta una consulta contra el handler y devuelve la respuesta.
func pedirResumen(t *testing.T, handler http.Handler, ruta string) *httptest.ResponseRecorder {
	t.Helper()
	grabadora := httptest.NewRecorder()
	handler.ServeHTTP(grabadora, httptest.NewRequest(http.MethodGet, ruta, nil))
	return grabadora
}

func TestResumenDevuelveElContratoDelTaller(t *testing.T) {
	handler := armarAPI(t, 4, 32, false)
	respuesta := pedirResumen(t, handler, "/resumen/1234?viajes=40")

	if respuesta.Code != http.StatusOK {
		t.Fatalf("codigo=%d, se esperaba 200: %s", respuesta.Code, respuesta.Body.String())
	}

	var cuerpo struct {
		IDTarjeta   string `json:"id_tarjeta"`
		TotalViajes int    `json:"total_viajes"`
		GastoTotal  int64  `json:"gasto_total"`
		DesdeCache  bool   `json:"desde_cache"`
	}
	if err := json.NewDecoder(respuesta.Body).Decode(&cuerpo); err != nil {
		t.Fatalf("cuerpo ilegible: %v", err)
	}
	if cuerpo.IDTarjeta != "1234" {
		t.Fatalf("id_tarjeta=%q, se esperaba \"1234\"", cuerpo.IDTarjeta)
	}
	if cuerpo.TotalViajes != 40 {
		t.Fatalf("total_viajes=%d, se esperaba 40", cuerpo.TotalViajes)
	}
	if cuerpo.GastoTotal <= 0 {
		t.Fatalf("gasto_total=%d, se esperaba un valor positivo", cuerpo.GastoTotal)
	}
	if cuerpo.DesdeCache {
		t.Fatal("la primera consulta no puede venir del cache")
	}
}

// El campo desde_cache es lo que el CSV usa para separar respuestas calculadas
// de respuestas servidas por la tactica R2.
func TestDesdeCacheDistingueLasRespuestas(t *testing.T) {
	handler := armarAPI(t, 4, 32, true)

	primera := pedirResumen(t, handler, "/resumen/1234?viajes=40")
	if cabecera := primera.Header().Get("X-Desde-Cache"); cabecera != "false" {
		t.Fatalf("X-Desde-Cache=%q en la primera consulta, se esperaba \"false\"", cabecera)
	}

	segunda := pedirResumen(t, handler, "/resumen/1234?viajes=40")
	if cabecera := segunda.Header().Get("X-Desde-Cache"); cabecera != "true" {
		t.Fatalf("X-Desde-Cache=%q en la segunda consulta, se esperaba \"true\"", cabecera)
	}

	var cuerpo struct {
		DesdeCache bool `json:"desde_cache"`
	}
	if err := json.NewDecoder(segunda.Body).Decode(&cuerpo); err != nil {
		t.Fatalf("cuerpo ilegible: %v", err)
	}
	if !cuerpo.DesdeCache {
		t.Fatal("desde_cache deberia ser true en la segunda consulta")
	}
}

func TestResumenUsaLosViajesPorDefecto(t *testing.T) {
	handler := armarAPI(t, 4, 32, false)
	respuesta := pedirResumen(t, handler, "/resumen/1234")

	var cuerpo struct {
		TotalViajes int `json:"total_viajes"`
	}
	if err := json.NewDecoder(respuesta.Body).Decode(&cuerpo); err != nil {
		t.Fatalf("cuerpo ilegible: %v", err)
	}
	if cuerpo.TotalViajes != 40 {
		t.Fatalf("total_viajes=%d, se esperaba el defecto 40", cuerpo.TotalViajes)
	}
}

func TestResumenRechazaEntradasInvalidas(t *testing.T) {
	handler := armarAPI(t, 4, 32, false)

	casos := []struct{ nombre, ruta string }{
		{"sin tarjeta", "/resumen/"},
		{"viajes en cero", "/resumen/1234?viajes=0"},
		{"viajes negativos", "/resumen/1234?viajes=-3"},
		{"viajes fuera de rango", "/resumen/1234?viajes=999999"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if respuesta := pedirResumen(t, handler, c.ruta); respuesta.Code != http.StatusBadRequest {
				t.Fatalf("codigo=%d, se esperaba 400", respuesta.Code)
			}
		})
	}
}

func TestResumenSoloAceptaGET(t *testing.T) {
	handler := armarAPI(t, 4, 32, false)
	grabadora := httptest.NewRecorder()
	handler.ServeHTTP(grabadora, httptest.NewRequest(http.MethodPost, "/resumen/1234", nil))

	if grabadora.Code != http.StatusMethodNotAllowed {
		t.Fatalf("codigo=%d, se esperaba 405", grabadora.Code)
	}
}

// Con la cola llena el servidor responde 503 de inmediato: el cliente lo
// registra como evento no procesado en vez de esperar hasta su timeout.
func TestServidorSaturadoResponde503(t *testing.T) {
	// 1 trabajador lento y cola de 1: la tercera solicitud simultanea no cabe.
	pool := usecase.NuevoPool(calculo.NuevaViajes(300*time.Millisecond),
		usecase.ConfigPool{Trabajadores: 1, CapacidadCola: 1})
	ctx, cancelar := context.WithCancel(context.Background())
	t.Cleanup(cancelar)
	pool.Iniciar(ctx)

	api := httpin.NuevaAPI(usecase.NuevoObtenerResumen(cache.Desactivada{}, pool),
		httpin.ConfigExpuesta{CostoCalculoMS: 300, ViajesPorDefecto: 40})
	handler := api.Rutas()

	// Dos solicitudes ocupan el trabajador y la cola.
	for i := 0; i < 2; i++ {
		go handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/resumen/1234", nil))
	}
	time.Sleep(50 * time.Millisecond)

	respuesta := pedirResumen(t, handler, "/resumen/9999")
	if respuesta.Code != http.StatusServiceUnavailable {
		t.Fatalf("codigo=%d, se esperaba 503 con la cola llena", respuesta.Code)
	}
	if respuesta.Header().Get("Retry-After") == "" {
		t.Fatal("una respuesta 503 deberia traer Retry-After")
	}
}

// GET /config es la verificacion en vivo que pide el enunciado.
func TestConfigPublicaTrabajadoresYEntradasDelCache(t *testing.T) {
	handler := armarAPI(t, 8, 128, true)

	// Una consulta para que el cache tenga una entrada.
	pedirResumen(t, handler, "/resumen/1234?viajes=40")

	grabadora := httptest.NewRecorder()
	handler.ServeHTTP(grabadora, httptest.NewRequest(http.MethodGet, "/config", nil))
	if grabadora.Code != http.StatusOK {
		t.Fatalf("codigo=%d, se esperaba 200", grabadora.Code)
	}

	var cuerpo struct {
		Trabajadores  int  `json:"trabajadores"`
		ColaMax       int  `json:"cola_max"`
		CacheActiva   bool `json:"cache_activa"`
		CacheEntradas int  `json:"cache_entradas"`
		CostoMS       int  `json:"costo_calculo_ms"`
	}
	if err := json.NewDecoder(grabadora.Body).Decode(&cuerpo); err != nil {
		t.Fatalf("cuerpo ilegible: %v", err)
	}
	if cuerpo.Trabajadores != 8 {
		t.Fatalf("trabajadores=%d, se esperaba 8", cuerpo.Trabajadores)
	}
	if cuerpo.ColaMax != 128 {
		t.Fatalf("cola_max=%d, se esperaba 128", cuerpo.ColaMax)
	}
	if !cuerpo.CacheActiva {
		t.Fatal("cache_activa deberia ser true")
	}
	if cuerpo.CacheEntradas != 1 {
		t.Fatalf("cache_entradas=%d, se esperaba 1", cuerpo.CacheEntradas)
	}
	if cuerpo.CostoMS != 5 {
		t.Fatalf("costo_calculo_ms=%d, se esperaba 5", cuerpo.CostoMS)
	}
}

func TestSaludResponde(t *testing.T) {
	handler := armarAPI(t, 2, 16, false)
	grabadora := httptest.NewRecorder()
	handler.ServeHTTP(grabadora, httptest.NewRequest(http.MethodGet, "/salud", nil))

	if grabadora.Code != http.StatusOK {
		t.Fatalf("codigo=%d, se esperaba 200", grabadora.Code)
	}
}
