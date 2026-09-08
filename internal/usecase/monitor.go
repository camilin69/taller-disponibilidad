package usecase

import (
	"context"
	"sync"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
)

// ConfigMonitor agrupa los parametros configurables del requisito R1.
// T, t, k y m nunca son constantes del codigo: llegan desde infra/config.
type ConfigMonitor struct {
	Intervalo time.Duration // T: periodo entre sondeos
	Timeout   time.Duration // t: tiempo maximo de espera del eco
}

// Monitor implementa la tactica Ping/Echo. Levanta una goroutine independiente
// por replica, de modo que un sondeo lento o colgado de una replica no retrasa
// el sondeo de las demas ni la atencion de consultas del dispatcher.
type Monitor struct {
	replicas  []domain.Replica
	registro  *RegistroReplicas
	sondeador Sondeador
	bitacora  Bitacora
	reloj     Reloj
	cfg       ConfigMonitor

	// Traza es un gancho opcional para observabilidad de cada sondeo
	// (se usa en modo depuracion y en pruebas).
	Traza func(replicaID string, exito bool, latencia time.Duration, err error)

	esperar sync.WaitGroup
}

// NuevoMonitor arma el caso de uso del monitor con sus puertos ya resueltos.
func NuevoMonitor(replicas []domain.Replica, registro *RegistroReplicas, sondeador Sondeador,
	bitacora Bitacora, reloj Reloj, cfg ConfigMonitor) *Monitor {
	if reloj == nil {
		reloj = RelojSistema{}
	}
	if cfg.Intervalo <= 0 {
		cfg.Intervalo = time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 300 * time.Millisecond
	}
	return &Monitor{
		replicas:  replicas,
		registro:  registro,
		sondeador: sondeador,
		bitacora:  bitacora,
		reloj:     reloj,
		cfg:       cfg,
	}
}

// Iniciar arranca el bucle de sondeo en segundo plano y retorna de inmediato:
// atender consultas jamas espera a que termine un ciclo de sondeo.
func (m *Monitor) Iniciar(ctx context.Context) {
	for _, r := range m.replicas {
		m.esperar.Add(1)
		go func(replica domain.Replica) {
			defer m.esperar.Done()
			m.bucleReplica(ctx, replica)
		}(r)
	}
}

// Esperar bloquea hasta que todas las goroutines de sondeo terminen (apagado).
func (m *Monitor) Esperar() { m.esperar.Wait() }

// bucleReplica sondea una replica cada T hasta que se cancele el contexto.
func (m *Monitor) bucleReplica(ctx context.Context, r domain.Replica) {
	// Primer sondeo inmediato: no se espera un periodo completo al arrancar.
	m.SondearUna(ctx, r)

	ticker := time.NewTicker(m.cfg.Intervalo)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.SondearUna(ctx, r)
		}
	}
}

// SondearUna ejecuta un ciclo de Ping/Echo sobre una replica: envia el ping con
// tiempo limite t, actualiza el registro y, si hubo transicion, la escribe en
// la bitacora. Es exportada para poder probarla paso a paso.
func (m *Monitor) SondearUna(ctx context.Context, r domain.Replica) *domain.CambioEstado {
	ctxSondeo, cancelar := context.WithTimeout(ctx, m.cfg.Timeout)
	defer cancelar()

	inicio := m.reloj.Ahora()
	err := m.sondeador.Sondear(ctxSondeo, r)
	fin := m.reloj.Ahora()
	latencia := fin.Sub(inicio)

	if m.Traza != nil {
		m.Traza(r.ID, err == nil, latencia, err)
	}

	// Si el apagado cancelo el contexto padre, no se contabiliza el fallo:
	// seria un falso positivo provocado por el cierre del dispatcher.
	if err != nil && ctx.Err() != nil {
		return nil
	}

	cambio := m.registro.RegistrarSondeo(r.ID, err == nil, latencia, fin)
	if cambio != nil && m.bitacora != nil {
		_ = m.bitacora.Registrar(*cambio)
	}
	return cambio
}
