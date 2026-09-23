package usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
)

// ResumenEstadisticas es la copia inmutable de los contadores (apta para JSON).
type ResumenEstadisticas struct {
	Total     int64            `json:"total"`
	Exitosas  int64            `json:"exitosas"`
	Fallidas  int64            `json:"fallidas"`
	Ganadoras map[string]int64 `json:"ganadoras"` // carreras ganadas por replica
}

// EstadisticasConsulta acumula contadores agregados de la redundancia activa de
// forma concurrente-segura. Alimenta GET /metricas y el panel React.
type EstadisticasConsulta struct {
	mu        sync.Mutex
	total     int64
	exitosas  int64
	fallidas  int64
	ganadoras map[string]int64
}

// NuevasEstadisticas construye el acumulador vacio.
func NuevasEstadisticas() *EstadisticasConsulta {
	return &EstadisticasConsulta{ganadoras: map[string]int64{}}
}

func (e *EstadisticasConsulta) registrarExito(replicaID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.total++
	e.exitosas++
	e.ganadoras[replicaID]++
}

func (e *EstadisticasConsulta) registrarFallo() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.total++
	e.fallidas++
}

// Instantanea devuelve una copia consistente de los contadores.
func (e *EstadisticasConsulta) Instantanea() ResumenEstadisticas {
	e.mu.Lock()
	defer e.mu.Unlock()
	copia := ResumenEstadisticas{Total: e.total, Exitosas: e.exitosas, Fallidas: e.fallidas,
		Ganadoras: make(map[string]int64, len(e.ganadoras))}
	for k, v := range e.ganadoras {
		copia.Ganadoras[k] = v
	}
	return copia
}

// ConsultarSaldo implementa la tactica de Redundancia Activa (hot spare):
// reenvia la consulta EN PARALELO a todas las replicas VIVA y responde con la
// primera respuesta valida (200 + saldo), descartando y cancelando las demas.
type ConsultarSaldo struct {
	registro     *RegistroReplicas
	pasarela     PasarelaSaldo
	timeout      time.Duration // tiempo maximo total de la carrera
	reloj        Reloj
	Estadisticas *EstadisticasConsulta
}

// NuevoConsultarSaldo arma el caso de uso de consulta con redundancia activa.
func NuevoConsultarSaldo(registro *RegistroReplicas, pasarela PasarelaSaldo, timeout time.Duration, reloj Reloj) *ConsultarSaldo {
	if reloj == nil {
		reloj = RelojSistema{}
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &ConsultarSaldo{
		registro:     registro,
		pasarela:     pasarela,
		timeout:      timeout,
		reloj:        reloj,
		Estadisticas: NuevasEstadisticas(),
	}
}

// Ejecutar corre la carrera entre replicas VIVA para una tarjeta.
func (c *ConsultarSaldo) Ejecutar(ctx context.Context, idTarjeta string) (domain.ResultadoConsulta, error) {
	if err := domain.ValidarIDTarjeta(idTarjeta); err != nil {
		return domain.ResultadoConsulta{}, err
	}

	vivas := c.registro.Vivas()
	if len(vivas) == 0 {
		c.Estadisticas.registrarFallo()
		return domain.ResultadoConsulta{}, domain.ErrSinReplicasVivas
	}

	ids := make([]string, 0, len(vivas))
	for _, r := range vivas {
		ids = append(ids, r.ID)
	}

	ctxCarrera, cancelar := context.WithTimeout(ctx, c.timeout)
	defer cancelar() // cancela las peticiones perdedoras en cuanto hay ganador

	inicio := c.reloj.Ahora()
	// Canales con buffer del tamano de la carrera: ninguna goroutine perdedora
	// queda bloqueada al escribir, por lo que no hay fuga de goroutines.
	ganadoras := make(chan domain.Saldo, len(vivas))
	fallos := make(chan error, len(vivas))

	for _, r := range vivas {
		go func(replica domain.Replica) {
			saldo, err := c.pasarela.ConsultarSaldo(ctxCarrera, replica, idTarjeta)
			if err != nil {
				fallos <- fmt.Errorf("replica %s: %w", replica.ID, err)
				return
			}
			if !saldo.EsValida() {
				fallos <- fmt.Errorf("replica %s: respuesta sin saldo valido", replica.ID)
				return
			}
			if saldo.ReplicaID == "" {
				saldo.ReplicaID = replica.ID
			}
			ganadoras <- saldo
		}(r)
	}

	var ultimoError error
	for pendientes := len(vivas); pendientes > 0; pendientes-- {
		select {
		case saldo := <-ganadoras:
			c.Estadisticas.registrarExito(saldo.ReplicaID)
			return domain.ResultadoConsulta{
				Saldo:          saldo,
				Latencia:       c.reloj.Ahora().Sub(inicio),
				ReplicasUsadas: ids,
			}, nil
		case err := <-fallos:
			ultimoError = err
		case <-ctxCarrera.Done():
			c.Estadisticas.registrarFallo()
			return domain.ResultadoConsulta{}, fmt.Errorf("%w: %v", domain.ErrTodasFallaron, ctxCarrera.Err())
		}
	}

	c.Estadisticas.registrarFallo()
	if ultimoError != nil {
		return domain.ResultadoConsulta{}, fmt.Errorf("%w: %v", domain.ErrTodasFallaron, ultimoError)
	}
	return domain.ResultadoConsulta{}, domain.ErrTodasFallaron
}
