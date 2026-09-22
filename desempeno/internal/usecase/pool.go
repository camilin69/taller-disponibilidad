package usecase

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/camilin69/taller-desempeno/internal/domain"
)

// ConfigPool parametriza la tactica "Introducir Concurrencia" (R1).
type ConfigPool struct {
	// Trabajadores (W) es cuantos calculos pueden estar en curso a la vez.
	// Es el parametro que se varia entre E0 (W=1) y E1/E2 (W=8).
	Trabajadores int
	// CapacidadCola acota cuantas solicitudes pueden esperar su turno. Al
	// llenarse, las nuevas se rechazan con domain.ErrColaLlena en vez de
	// crecer sin limite hasta agotar la memoria del proceso.
	CapacidadCola int
}

// Validar comprueba que la configuracion del pool sea coherente.
func (c ConfigPool) Validar() error {
	if c.Trabajadores < 1 {
		return fmt.Errorf("TRABAJADORES debe ser >= 1, se configuro %d", c.Trabajadores)
	}
	if c.CapacidadCola < 1 {
		return fmt.Errorf("COLA_MAX debe ser >= 1, se configuro %d", c.CapacidadCola)
	}
	return nil
}

// Pool es la implementacion de la tactica "Introducir Concurrencia": un
// conjunto FIJO de W trabajadores que consumen tareas de una cola acotada.
//
// El numero de trabajadores es el limite real de paralelismo del servicio: no
// se delega en el servidor HTTP (que arrancaria una goroutine por peticion y
// haria imposible medir W=1), sino que se configura aqui explicitamente.
type Pool struct {
	tareas       chan tarea
	calculadora  Calculadora
	trabajadores int
	capacidad    int

	ocupados    atomic.Int64
	encoladas   atomic.Int64
	rechazadas  atomic.Int64
	abandonadas atomic.Int64
	completadas atomic.Int64
	esperaTotal atomic.Int64 // microsegundos acumulados de espera en cola
	iniciar     sync.Once
	activos     sync.WaitGroup
}

// tarea es una unidad de trabajo en la cola: que calcular y a donde responder.
type tarea struct {
	ctx       context.Context
	clave     domain.ClaveResumen
	encolada  time.Time
	respuesta chan resultadoTarea
}

// resultadoTarea es lo que el trabajador devuelve a quien encolo la tarea.
type resultadoTarea struct {
	resumen    domain.Resumen
	esperaCola time.Duration
	err        error
}

// EstadoPool es la foto instantanea que publica GET /config para verificar en
// vivo que la concurrencia esta funcionando.
type EstadoPool struct {
	Trabajadores      int     `json:"trabajadores"`
	Ocupados          int64   `json:"trabajadores_ocupados"`
	CapacidadCola     int     `json:"cola_max"`
	EnCola            int     `json:"cola_actual"`
	Encoladas         int64   `json:"solicitudes_encoladas"`
	Completadas       int64   `json:"solicitudes_completadas"`
	Rechazadas        int64   `json:"solicitudes_rechazadas"`
	Abandonadas       int64   `json:"solicitudes_abandonadas"`
	EsperaColaMediaMS float64 `json:"espera_cola_media_ms"`
}

// NuevoPool construye el pool. No arranca los trabajadores: eso lo hace
// Iniciar, para que el ensamblaje controle el ciclo de vida.
func NuevoPool(calculadora Calculadora, cfg ConfigPool) *Pool {
	return &Pool{
		tareas:       make(chan tarea, cfg.CapacidadCola),
		calculadora:  calculadora,
		trabajadores: cfg.Trabajadores,
		capacidad:    cfg.CapacidadCola,
	}
}

// Iniciar arranca los W trabajadores en segundo plano (no bloquea). Los
// trabajadores viven hasta que el contexto se cancela.
func (p *Pool) Iniciar(ctx context.Context) {
	p.iniciar.Do(func() {
		for i := 0; i < p.trabajadores; i++ {
			p.activos.Add(1)
			go p.trabajar(ctx)
		}
	})
}

// Esperar bloquea hasta que todos los trabajadores terminaron tras cancelarse
// el contexto. Lo usan las pruebas para no dejar goroutines sueltas.
func (p *Pool) Esperar() { p.activos.Wait() }

// Ejecutar encola el calculo y espera su resultado.
//
// Devuelve domain.ErrColaLlena sin esperar cuando la cola esta llena: mas vale
// rechazar de inmediato (el cliente lo cuenta como evento no procesado) que
// aceptar una solicitud que de todos modos vencera su tiempo limite.
func (p *Pool) Ejecutar(ctx context.Context, clave domain.ClaveResumen) (domain.Resumen, time.Duration, error) {
	t := tarea{
		ctx:      ctx,
		clave:    clave,
		encolada: time.Now(),
		// Buffer 1: el trabajador nunca se bloquea al responder, aunque quien
		// encolo ya se haya rendido por timeout.
		respuesta: make(chan resultadoTarea, 1),
	}

	select {
	case p.tareas <- t:
		p.encoladas.Add(1)
	default:
		p.rechazadas.Add(1)
		return domain.Resumen{}, 0, domain.ErrColaLlena
	}

	select {
	case res := <-t.respuesta:
		return res.resumen, res.esperaCola, res.err
	case <-ctx.Done():
		return domain.Resumen{}, time.Since(t.encolada), ctx.Err()
	}
}

// Estado devuelve los contadores del pool para GET /config.
func (p *Pool) Estado() EstadoPool {
	completadas := p.completadas.Load()
	media := 0.0
	if completadas > 0 {
		media = float64(p.esperaTotal.Load()) / float64(completadas) / 1000.0
	}
	return EstadoPool{
		Trabajadores:      p.trabajadores,
		Ocupados:          p.ocupados.Load(),
		CapacidadCola:     p.capacidad,
		EnCola:            len(p.tareas),
		Encoladas:         p.encoladas.Load(),
		Completadas:       completadas,
		Rechazadas:        p.rechazadas.Load(),
		Abandonadas:       p.abandonadas.Load(),
		EsperaColaMediaMS: media,
	}
}

// trabajar es el bucle de un trabajador: toma tareas de la cola hasta que el
// contexto se cancela.
func (p *Pool) trabajar(ctx context.Context) {
	defer p.activos.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-p.tareas:
			p.atender(t)
		}
	}
}

// atender ejecuta una tarea y publica su resultado.
func (p *Pool) atender(t tarea) {
	espera := time.Since(t.encolada)

	// Quien encolo pudo rendirse mientras la tarea esperaba su turno. Gastar un
	// trabajador en un calculo que nadie va a leer solo retrasa a los demas:
	// en E0 (W=1) es la diferencia entre recuperarse o arrastrar la cola.
	if t.ctx.Err() != nil {
		p.abandonadas.Add(1)
		t.respuesta <- resultadoTarea{esperaCola: espera, err: t.ctx.Err()}
		return
	}

	p.ocupados.Add(1)
	resumen, err := p.calculadora.Calcular(t.ctx, t.clave)
	p.ocupados.Add(-1)

	p.completadas.Add(1)
	p.esperaTotal.Add(espera.Microseconds())
	t.respuesta <- resultadoTarea{resumen: resumen, esperaCola: espera, err: err}
}
