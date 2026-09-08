package usecase

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
)

// bitacoraEnMemoria es un doble de prueba del puerto Bitacora.
type bitacoraEnMemoria struct {
	mu      sync.Mutex
	cambios []domain.CambioEstado
}

func (b *bitacoraEnMemoria) Registrar(c domain.CambioEstado) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cambios = append(b.cambios, c)
	return nil
}

func (b *bitacoraEnMemoria) Ultimas(n int) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	lineas := make([]string, 0, len(b.cambios))
	for _, c := range b.cambios {
		lineas = append(lineas, c.Linea())
	}
	if n > 0 && len(lineas) > n {
		lineas = lineas[len(lineas)-n:]
	}
	return lineas
}

func (b *bitacoraEnMemoria) total() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.cambios)
}

// sondeadorFalso permite programar el resultado del ping por replica.
type sondeadorFalso struct {
	mu       sync.Mutex
	caidas   map[string]bool          // replicas que fallan el ping
	demoras  map[string]time.Duration // demora simulada antes de responder
	llamadas map[string]int
}

func nuevoSondeadorFalso() *sondeadorFalso {
	return &sondeadorFalso{caidas: map[string]bool{}, demoras: map[string]time.Duration{}, llamadas: map[string]int{}}
}

func (s *sondeadorFalso) tumbar(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.caidas[id] = true
}

func (s *sondeadorFalso) levantar(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.caidas[id] = false
}

func (s *sondeadorFalso) Sondear(ctx context.Context, r domain.Replica) error {
	s.mu.Lock()
	caida := s.caidas[r.ID]
	demora := s.demoras[r.ID]
	s.llamadas[r.ID]++
	s.mu.Unlock()

	if demora > 0 {
		select {
		case <-time.After(demora):
		case <-ctx.Done():
			return ctx.Err() // se agoto el tiempo limite t
		}
	}
	if caida {
		return errors.New("conexion rechazada")
	}
	return ctx.Err()
}

// pasarelaFalsa simula el endpoint /saldo de cada replica.
type pasarelaFalsa struct {
	mu       sync.Mutex
	demoras  map[string]time.Duration
	errores  map[string]error
	invalida map[string]bool
	saldo    int64
	llamadas map[string]int
}

func nuevaPasarelaFalsa() *pasarelaFalsa {
	return &pasarelaFalsa{demoras: map[string]time.Duration{}, errores: map[string]error{},
		invalida: map[string]bool{}, saldo: 15000, llamadas: map[string]int{}}
}

func (p *pasarelaFalsa) ConsultarSaldo(ctx context.Context, r domain.Replica, idTarjeta string) (domain.Saldo, error) {
	p.mu.Lock()
	demora := p.demoras[r.ID]
	err := p.errores[r.ID]
	invalida := p.invalida[r.ID]
	saldo := p.saldo
	p.llamadas[r.ID]++
	p.mu.Unlock()

	if demora > 0 {
		select {
		case <-time.After(demora):
		case <-ctx.Done():
			return domain.Saldo{}, ctx.Err()
		}
	}
	if err != nil {
		return domain.Saldo{}, err
	}
	if invalida {
		return domain.Saldo{ReplicaID: "", Valor: -1}, nil
	}
	return domain.Saldo{ReplicaID: r.ID, IDTarjeta: idTarjeta, Valor: saldo}, nil
}

func (p *pasarelaFalsa) vecesLlamada(id string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.llamadas[id]
}

// relojFijo permite controlar el tiempo en pruebas deterministas.
type relojFijo struct {
	mu     sync.Mutex
	actual time.Time
}

func (r *relojFijo) Ahora() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.actual
}

func (r *relojFijo) avanzar(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actual = r.actual.Add(d)
}
