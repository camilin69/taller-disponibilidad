// Package ensamblaje es el composition root del dispatcher: construye los
// adaptadores concretos y los inyecta en los casos de uso. Al estar aislado
// aqui, tanto el binario de produccion como las pruebas de integracion arman
// exactamente el mismo sistema.
package ensamblaje

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/adapter/bitacora"
	"github.com/camilin69/taller-disponibilidad/internal/adapter/gateway"
	"github.com/camilin69/taller-disponibilidad/internal/adapter/httpin"
	"github.com/camilin69/taller-disponibilidad/internal/infra/config"
	"github.com/camilin69/taller-disponibilidad/internal/usecase"
)

// Sistema agrupa las piezas ya cableadas del dispatcher.
type Sistema struct {
	Config   config.Dispatcher
	Registro *usecase.RegistroReplicas
	Monitor  *usecase.Monitor
	Consulta *usecase.ConsultarSaldo
	Bitacora *bitacora.Archivo
	API      http.Handler
}

// ArmarDispatcher cablea bitacora, adaptadores HTTP de salida, registro de
// estado, monitor Ping/Echo y caso de uso de consulta con redundancia activa.
func ArmarDispatcher(cfg config.Dispatcher) (*Sistema, error) {
	if err := cfg.Validar(); err != nil {
		return nil, err
	}

	bit, err := bitacora.NuevoArchivo(cfg.ArchivoBitacora, true)
	if err != nil {
		return nil, err
	}

	registro := usecase.NuevoRegistroReplicas(cfg.Replicas, cfg.EstadoInicial, cfg.K, cfg.M)
	monitor := usecase.NuevoMonitor(cfg.Replicas, registro, gateway.NuevoSondeadorHTTP(4), bit,
		usecase.RelojSistema{}, usecase.ConfigMonitor{Intervalo: cfg.IntervaloSondeo, Timeout: cfg.TimeoutSondeo})
	if cfg.TrazaSondeos {
		monitor.Traza = func(replicaID string, exito bool, latencia time.Duration, err error) {
			if exito {
				log.Printf("ping %s OK (%.1f ms)", replicaID, float64(latencia.Microseconds())/1000)
			} else {
				log.Printf("ping %s FALLO: %v", replicaID, err)
			}
		}
	}
	consulta := usecase.NuevoConsultarSaldo(registro, gateway.NuevaPasarelaSaldoHTTP(64),
		cfg.TimeoutConsulta, usecase.RelojSistema{})

	api := httpin.NuevoDispatcher(consulta, registro, bit, httpin.ConfigExpuesta{
		IntervaloSondeoMS:   cfg.IntervaloSondeo.Milliseconds(),
		TimeoutSondeoMS:     cfg.TimeoutSondeo.Milliseconds(),
		K:                   cfg.K,
		M:                   cfg.M,
		TimeoutConsultaMS:   cfg.TimeoutConsulta.Milliseconds(),
		DeteccionEsperadaMS: cfg.DeteccionEsperada().Milliseconds(),
	})

	return &Sistema{
		Config:   cfg,
		Registro: registro,
		Monitor:  monitor,
		Consulta: consulta,
		Bitacora: bit,
		API:      api.Rutas(),
	}, nil
}

// IniciarMonitor arranca el sondeo en segundo plano (no bloquea).
func (s *Sistema) IniciarMonitor(ctx context.Context) { s.Monitor.Iniciar(ctx) }

// Cerrar libera los recursos del sistema (archivo de bitacora).
func (s *Sistema) Cerrar() error { return s.Bitacora.Cerrar() }
