package usecase_test

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/camilin69/taller-desempeno/internal/domain"
)

// calculadoraDoble es una Calculadora controlable: tarda lo que se le diga y
// cuenta cuantos calculos corrieron a la vez. Es lo que permite comprobar que
// W es de verdad el limite de paralelismo sin esperar 200 ms por solicitud.
type calculadoraDoble struct {
	duracion time.Duration

	mu            sync.Mutex
	enCurso       int
	picoEnCurso   int
	llamadas      atomic.Int64
	porClave      map[domain.ClaveResumen]int
	errorDevuelto error
}

func nuevaCalculadoraDoble(duracion time.Duration) *calculadoraDoble {
	return &calculadoraDoble{
		duracion: duracion,
		porClave: map[domain.ClaveResumen]int{},
	}
}

func (c *calculadoraDoble) Calcular(ctx context.Context, clave domain.ClaveResumen) (domain.Resumen, error) {
	c.llamadas.Add(1)

	c.mu.Lock()
	c.enCurso++
	if c.enCurso > c.picoEnCurso {
		c.picoEnCurso = c.enCurso
	}
	c.porClave[clave]++
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.enCurso--
		c.mu.Unlock()
	}()

	if c.duracion > 0 {
		temporizador := time.NewTimer(c.duracion)
		defer temporizador.Stop()
		select {
		case <-temporizador.C:
		case <-ctx.Done():
			return domain.Resumen{}, ctx.Err()
		}
	}
	if c.errorDevuelto != nil {
		return domain.Resumen{}, c.errorDevuelto
	}

	return domain.Resumen{
		IDTarjeta:   clave.IDTarjeta,
		TotalViajes: clave.Viajes,
		GastoTotal:  domain.SumarViajes(clave.IDTarjeta, clave.Viajes),
	}, nil
}

// pico devuelve cuantos calculos llegaron a correr simultaneamente.
func (c *calculadoraDoble) pico() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.picoEnCurso
}

// vecesCalculada indica cuantas veces se calculo una clave concreta.
func (c *calculadoraDoble) vecesCalculada(clave domain.ClaveResumen) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.porClave[clave]
}

// cacheDoble es un cache en memoria minimo para las pruebas del caso de uso.
type cacheDoble struct {
	mu       sync.RWMutex
	entradas map[domain.ClaveResumen]domain.Resumen
}

func nuevoCacheDoble() *cacheDoble {
	return &cacheDoble{entradas: map[domain.ClaveResumen]domain.Resumen{}}
}

func (c *cacheDoble) Obtener(clave domain.ClaveResumen) (domain.Resumen, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	r, ok := c.entradas[clave]
	return r, ok
}

func (c *cacheDoble) Guardar(clave domain.ClaveResumen, resumen domain.Resumen) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entradas[clave] = resumen
}

func (c *cacheDoble) Entradas() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entradas)
}

func (c *cacheDoble) Activa() bool { return true }

// cacheApagado representa la configuracion de E0 y E1: la tactica R2 apagada.
type cacheApagado struct{}

func (cacheApagado) Obtener(domain.ClaveResumen) (domain.Resumen, bool) {
	return domain.Resumen{}, false
}
func (cacheApagado) Guardar(domain.ClaveResumen, domain.Resumen) {}
func (cacheApagado) Entradas() int                               { return 0 }
func (cacheApagado) Activa() bool                                { return false }
