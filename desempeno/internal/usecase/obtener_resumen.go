package usecase

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/camilin69/taller-desempeno/internal/domain"
)

// ObtenerResumen es el caso de uso central del taller. Encadena las dos
// tacticas en el orden que exige el enunciado:
//
//  1. Mantener Multiples Copias de Datos (R2): si el resumen ya se calculo,
//     se responde de inmediato sin tocar el pool.
//  2. Introducir Concurrencia (R1): si no esta en cache, se encola en el pool
//     de W trabajadores, se calcula, se guarda y se responde.
//
// El orden importa: consultar el cache ANTES de encolar es lo que evita que un
// acierto ocupe un trabajador. Si el cache se consultara dentro del trabajador,
// las respuestas instantaneas seguirian haciendo fila detras de los calculos.
type ObtenerResumen struct {
	cache Cache
	pool  *Pool

	aciertos atomic.Int64
	fallos   atomic.Int64
}

// Traza son los datos de observabilidad de una consulta atendida.
type Traza struct {
	DesdeCache bool
	EsperaCola time.Duration // cuanto espero la solicitud su turno en la cola
	Total      time.Duration // tiempo total dentro del servidor
}

// EstadoCache son los contadores del cache que publica GET /config.
type EstadoCache struct {
	Activa   bool    `json:"cache_activa"`
	Entradas int     `json:"cache_entradas"`
	Aciertos int64   `json:"cache_aciertos"`
	Fallos   int64   `json:"cache_fallos"`
	Tasa     float64 `json:"cache_tasa_aciertos"`
}

// NuevoObtenerResumen cablea el caso de uso con su cache y su pool.
func NuevoObtenerResumen(cache Cache, pool *Pool) *ObtenerResumen {
	return &ObtenerResumen{cache: cache, pool: pool}
}

// Ejecutar resuelve una consulta de resumen mensual.
func (uc *ObtenerResumen) Ejecutar(ctx context.Context, clave domain.ClaveResumen) (domain.Resumen, Traza, error) {
	inicio := time.Now()

	// --- R2: el cache responde sin calcular ---
	if resumen, ok := uc.cache.Obtener(clave); ok {
		uc.aciertos.Add(1)
		resumen.DesdeCache = true
		return resumen, Traza{DesdeCache: true, Total: time.Since(inicio)}, nil
	}
	uc.fallos.Add(1)

	// --- R1: el calculo costoso pasa por el pool de trabajadores ---
	resumen, espera, err := uc.pool.Ejecutar(ctx, clave)
	if err != nil {
		return domain.Resumen{}, Traza{EsperaCola: espera, Total: time.Since(inicio)}, err
	}

	resumen.DesdeCache = false
	uc.cache.Guardar(clave, resumen)
	return resumen, Traza{EsperaCola: espera, Total: time.Since(inicio)}, nil
}

// EstadoCache devuelve los contadores del cache.
func (uc *ObtenerResumen) EstadoCache() EstadoCache {
	aciertos := uc.aciertos.Load()
	fallos := uc.fallos.Load()
	tasa := 0.0
	if total := aciertos + fallos; total > 0 {
		tasa = 100 * float64(aciertos) / float64(total)
	}
	return EstadoCache{
		Activa:   uc.cache.Activa(),
		Entradas: uc.cache.Entradas(),
		Aciertos: aciertos,
		Fallos:   fallos,
		Tasa:     tasa,
	}
}

// EstadoPool devuelve los contadores del pool de trabajadores.
func (uc *ObtenerResumen) EstadoPool() EstadoPool { return uc.pool.Estado() }
