// Package ensamblaje es el composition root del servidor: construye los
// adaptadores concretos y los inyecta en los casos de uso. Al estar aislado
// aqui, tanto el binario de produccion como las pruebas de integracion arman
// exactamente el mismo sistema.
package ensamblaje

import (
	"context"
	"net/http"

	"github.com/camilin69/taller-desempeno/internal/adapter/cache"
	"github.com/camilin69/taller-desempeno/internal/adapter/calculo"
	"github.com/camilin69/taller-desempeno/internal/adapter/httpin"
	"github.com/camilin69/taller-desempeno/internal/infra/config"
	"github.com/camilin69/taller-desempeno/internal/usecase"
)

// Sistema agrupa las piezas ya cableadas del servidor.
type Sistema struct {
	Config  config.Servidor
	Pool    *usecase.Pool
	Resumen *usecase.ObtenerResumen
	API     http.Handler
}

// ArmarServidor cablea la calculadora costosa, el pool de trabajadores (R1),
// el cache (R2) y el adaptador HTTP de entrada.
func ArmarServidor(cfg config.Servidor) (*Sistema, error) {
	if err := cfg.Validar(); err != nil {
		return nil, err
	}

	// R2: encender o apagar la tactica es elegir la implementacion del puerto,
	// no poner un "if cacheActiva" dentro del caso de uso.
	var almacen usecase.Cache = cache.Desactivada{}
	if cfg.CacheActiva {
		almacen = cache.NuevaMemoria()
	}

	// R1: el pool es el unico lugar donde ocurre el calculo costoso, asi que W
	// es de verdad el limite de paralelismo del servicio.
	pool := usecase.NuevoPool(calculo.NuevaViajes(cfg.CostoCalculo), usecase.ConfigPool{
		Trabajadores:  cfg.Trabajadores,
		CapacidadCola: cfg.CapacidadCola,
	})

	resumen := usecase.NuevoObtenerResumen(almacen, pool)

	api := httpin.NuevaAPI(resumen, httpin.ConfigExpuesta{
		CostoCalculoMS:   cfg.CostoCalculo.Milliseconds(),
		ViajesPorDefecto: cfg.ViajesPorDefecto,
	})
	api.Registro = httpin.OpcionesRegistro{Activo: cfg.LogPeticiones, IncluirVigilancia: cfg.LogVigilancia}

	return &Sistema{
		Config:  cfg,
		Pool:    pool,
		Resumen: resumen,
		API:     api.Rutas(),
	}, nil
}

// IniciarTrabajadores arranca el pool en segundo plano (no bloquea).
func (s *Sistema) IniciarTrabajadores(ctx context.Context) { s.Pool.Iniciar(ctx) }
