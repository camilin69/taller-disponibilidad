package usecase

import (
	"fmt"
	"sync"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
)

// SaludReplica es la instantanea observable del estado de una replica; es lo
// que alimenta GET /estado y la interfaz React.
type SaludReplica struct {
	ID                 string        `json:"replica_id"`
	URLBase            string        `json:"url_base"`
	Estado             domain.Estado `json:"estado"`
	FallosConsecutivos int           `json:"fallos_consecutivos"`
	ExitosConsecutivos int           `json:"exitos_consecutivos"`
	TotalSondeos       int64         `json:"total_sondeos"`
	TotalFallos        int64         `json:"total_fallos"`
	UltimoSondeo       *time.Time    `json:"ultimo_sondeo,omitempty"`
	UltimoCambio       *time.Time    `json:"ultimo_cambio,omitempty"`
	UltimaLatenciaMS   float64       `json:"ultima_latencia_ms"`
}

// entradaRegistro es el estado mutable interno de cada replica.
type entradaRegistro struct {
	replica            domain.Replica
	estado             domain.Estado
	fallosConsecutivos int
	exitosConsecutivos int
	totalSondeos       int64
	totalFallos        int64
	ultimoSondeo       time.Time
	ultimoCambio       time.Time
	ultimaLatencia     time.Duration
}

// RegistroReplicas mantiene, de forma concurrente-segura, el estado VIVA/CAIDA
// de cada replica y aplica las reglas de histeresis k (fallos para caer) y
// m (exitos para revivir). Es la pieza de negocio del requisito R1.
type RegistroReplicas struct {
	mu       sync.RWMutex
	entradas map[string]*entradaRegistro
	orden    []string // orden estable de presentacion (el de configuracion)
	k        int      // fallos consecutivos para marcar CAIDA
	m        int      // exitos consecutivos para marcar VIVA de nuevo
}

// NuevoRegistroReplicas construye el registro con el estado inicial indicado.
// k y m se normalizan a un minimo de 1 para evitar configuraciones absurdas.
func NuevoRegistroReplicas(replicas []domain.Replica, estadoInicial domain.Estado, k, m int) *RegistroReplicas {
	if k < 1 {
		k = 1
	}
	if m < 1 {
		m = 1
	}
	reg := &RegistroReplicas{entradas: make(map[string]*entradaRegistro, len(replicas)), k: k, m: m}
	for _, r := range replicas {
		if _, existe := reg.entradas[r.ID]; existe {
			continue // ids duplicados: se conserva el primero
		}
		reg.entradas[r.ID] = &entradaRegistro{replica: r, estado: estadoInicial}
		reg.orden = append(reg.orden, r.ID)
	}
	return reg
}

// K expone el umbral de fallos consecutivos para marcar CAIDA.
func (reg *RegistroReplicas) K() int { return reg.k }

// M expone el umbral de exitos consecutivos para volver a VIVA.
func (reg *RegistroReplicas) M() int { return reg.m }

// RegistrarSondeo aplica el resultado de un ping. Devuelve un *CambioEstado no
// nulo unicamente cuando se cruza el umbral k o m, es decir cuando hay que
// escribir en la bitacora.
func (reg *RegistroReplicas) RegistrarSondeo(id string, exito bool, latencia time.Duration, momento time.Time) *domain.CambioEstado {
	reg.mu.Lock()
	defer reg.mu.Unlock()

	e, ok := reg.entradas[id]
	if !ok {
		return nil // sondeo de una replica desconocida: se ignora
	}
	e.totalSondeos++
	e.ultimoSondeo = momento
	if exito {
		e.ultimaLatencia = latencia
		e.exitosConsecutivos++
		e.fallosConsecutivos = 0
		if e.estado == domain.EstadoCaida && e.exitosConsecutivos >= reg.m {
			return reg.transicion(e, domain.EstadoViva, momento,
				fmt.Sprintf("%d exitos consecutivos de ping", reg.m))
		}
		return nil
	}

	e.totalFallos++
	e.fallosConsecutivos++
	e.exitosConsecutivos = 0
	if e.estado == domain.EstadoViva && e.fallosConsecutivos >= reg.k {
		return reg.transicion(e, domain.EstadoCaida, momento,
			fmt.Sprintf("%d fallos consecutivos de ping", reg.k))
	}
	return nil
}

// transicion cambia el estado, reinicia los contadores y arma el evento.
// Debe invocarse con el mutex de escritura tomado.
func (reg *RegistroReplicas) transicion(e *entradaRegistro, nuevo domain.Estado, momento time.Time, motivo string) *domain.CambioEstado {
	anterior := e.estado
	e.estado = nuevo
	e.ultimoCambio = momento
	e.fallosConsecutivos = 0
	e.exitosConsecutivos = 0
	return &domain.CambioEstado{
		Momento:   momento,
		ReplicaID: e.replica.ID,
		Anterior:  anterior,
		Nuevo:     nuevo,
		Motivo:    motivo,
	}
}

// Vivas devuelve las replicas marcadas como VIVA, en orden de configuracion.
// Es la lista sobre la que la redundancia activa hace fan-out.
func (reg *RegistroReplicas) Vivas() []domain.Replica {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	vivas := make([]domain.Replica, 0, len(reg.orden))
	for _, id := range reg.orden {
		if e := reg.entradas[id]; e != nil && e.estado == domain.EstadoViva {
			vivas = append(vivas, e.replica)
		}
	}
	return vivas
}

// Todas devuelve todas las replicas registradas, en orden de configuracion.
func (reg *RegistroReplicas) Todas() []domain.Replica {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	todas := make([]domain.Replica, 0, len(reg.orden))
	for _, id := range reg.orden {
		todas = append(todas, reg.entradas[id].replica)
	}
	return todas
}

// EstadoDe devuelve el estado actual de una replica.
func (reg *RegistroReplicas) EstadoDe(id string) (domain.Estado, bool) {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	e, ok := reg.entradas[id]
	if !ok {
		return "", false
	}
	return e.estado, true
}

// Instantanea es el mapa simple {"A":"VIVA","B":"CAIDA"} exigido por el taller.
func (reg *RegistroReplicas) Instantanea() map[string]domain.Estado {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	mapa := make(map[string]domain.Estado, len(reg.entradas))
	for id, e := range reg.entradas {
		mapa[id] = e.estado
	}
	return mapa
}

// Detalle devuelve la instantanea enriquecida (orden de configuracion) que
// consume la interfaz React.
func (reg *RegistroReplicas) Detalle() []SaludReplica {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	detalle := make([]SaludReplica, 0, len(reg.orden))
	for _, id := range reg.orden {
		e := reg.entradas[id]
		s := SaludReplica{
			ID:                 e.replica.ID,
			URLBase:            e.replica.URLBase,
			Estado:             e.estado,
			FallosConsecutivos: e.fallosConsecutivos,
			ExitosConsecutivos: e.exitosConsecutivos,
			TotalSondeos:       e.totalSondeos,
			TotalFallos:        e.totalFallos,
			UltimaLatenciaMS:   float64(e.ultimaLatencia.Microseconds()) / 1000.0,
		}
		if !e.ultimoSondeo.IsZero() {
			t := e.ultimoSondeo
			s.UltimoSondeo = &t
		}
		if !e.ultimoCambio.IsZero() {
			t := e.ultimoCambio
			s.UltimoCambio = &t
		}
		detalle = append(detalle, s)
	}
	return detalle
}
