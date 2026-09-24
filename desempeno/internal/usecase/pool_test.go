package usecase_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/camilin69/taller-desempeno/internal/domain"
	"github.com/camilin69/taller-desempeno/internal/usecase"
)

// clave de prueba reutilizable.
func clave(id string) domain.ClaveResumen {
	return domain.ClaveResumen{IDTarjeta: id, Viajes: 40}
}

// Con W=1 el pool debe serializar: nunca dos calculos a la vez. Es la
// condicion que hace de E0 una linea base honesta.
func TestPoolConUnTrabajadorSerializa(t *testing.T) {
	calc := nuevaCalculadoraDoble(20 * time.Millisecond)
	pool := usecase.NuevoPool(calc, usecase.ConfigPool{Trabajadores: 1, CapacidadCola: 32})

	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()
	pool.Iniciar(ctx)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if _, _, err := pool.Ejecutar(ctx, clave("tarjeta")); err != nil {
				t.Errorf("solicitud %d fallo: %v", n, err)
			}
		}(i)
	}
	wg.Wait()

	if pico := calc.pico(); pico != 1 {
		t.Fatalf("con W=1 el pico de concurrencia deberia ser 1, fue %d", pico)
	}
}

// Con W=4 el pool debe llegar a 4 calculos simultaneos: es la evidencia de que
// la tactica R1 esta activa y de que W manda de verdad.
func TestPoolAlcanzaLaConcurrenciaConfigurada(t *testing.T) {
	const trabajadores = 4
	calc := nuevaCalculadoraDoble(60 * time.Millisecond)
	pool := usecase.NuevoPool(calc, usecase.ConfigPool{Trabajadores: trabajadores, CapacidadCola: 32})

	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()
	pool.Iniciar(ctx)

	var wg sync.WaitGroup
	for i := 0; i < trabajadores*2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = pool.Ejecutar(ctx, clave("tarjeta"))
		}()
	}
	wg.Wait()

	if pico := calc.pico(); pico != trabajadores {
		t.Fatalf("con W=%d el pico de concurrencia deberia ser %d, fue %d",
			trabajadores, trabajadores, pico)
	}
}

// Mas trabajadores deben traducirse en menos tiempo total para la misma
// rafaga: es la mejora de throughput que compara el taller entre E0 y E1.
func TestPoolMasTrabajadoresTerminanAntes(t *testing.T) {
	medir := func(trabajadores int) time.Duration {
		calc := nuevaCalculadoraDoble(30 * time.Millisecond)
		pool := usecase.NuevoPool(calc, usecase.ConfigPool{Trabajadores: trabajadores, CapacidadCola: 64})
		ctx, cancelar := context.WithCancel(context.Background())
		defer cancelar()
		pool.Iniciar(ctx)

		inicio := time.Now()
		var wg sync.WaitGroup
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _, _ = pool.Ejecutar(ctx, clave("tarjeta"))
			}()
		}
		wg.Wait()
		return time.Since(inicio)
	}

	conUno := medir(1)
	conOcho := medir(8)
	if conOcho >= conUno {
		t.Fatalf("W=8 (%v) deberia terminar antes que W=1 (%v)", conOcho, conUno)
	}
}

// Cuando la cola se llena, las solicitudes nuevas se rechazan de inmediato en
// vez de acumularse sin limite: son los eventos no procesados de la teoria.
func TestPoolRechazaCuandoLaColaSeLlena(t *testing.T) {
	calc := nuevaCalculadoraDoble(200 * time.Millisecond)
	pool := usecase.NuevoPool(calc, usecase.ConfigPool{Trabajadores: 1, CapacidadCola: 2})

	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()
	pool.Iniciar(ctx)

	// Se lanzan muchas mas solicitudes de las que caben: 1 en curso + 2 en cola.
	var (
		mu         sync.Mutex
		rechazadas int
		wg         sync.WaitGroup
	)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := pool.Ejecutar(ctx, clave("tarjeta"))
			if errors.Is(err, domain.ErrColaLlena) {
				mu.Lock()
				rechazadas++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if rechazadas == 0 {
		t.Fatal("con cola de 2 y 12 solicitudes deberia haber rechazos")
	}
	if estado := pool.Estado(); estado.Rechazadas == 0 {
		t.Fatal("el contador de rechazadas del pool deberia reflejar los rechazos")
	}
}

// Si quien pidio el resumen se rinde, el trabajador no debe gastar su turno en
// un calculo que nadie va a leer.
func TestPoolDescartaTareasAbandonadas(t *testing.T) {
	calc := nuevaCalculadoraDoble(80 * time.Millisecond)
	pool := usecase.NuevoPool(calc, usecase.ConfigPool{Trabajadores: 1, CapacidadCola: 8})

	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()
	pool.Iniciar(ctx)

	// La primera ocupa al unico trabajador durante 80 ms.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _, _ = pool.Ejecutar(ctx, clave("ocupante"))
	}()
	time.Sleep(10 * time.Millisecond)

	// Esta se encola pero su contexto vence antes de llegar a un trabajador.
	ctxCorto, cancelarCorto := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancelarCorto()
	_, _, err := pool.Ejecutar(ctxCorto, clave("abandonada"))
	if err == nil {
		t.Fatal("se esperaba un error de contexto vencido")
	}
	wg.Wait()

	// Se le da tiempo al trabajador de tomar la tarea abandonada y descartarla.
	time.Sleep(50 * time.Millisecond)
	if veces := calc.vecesCalculada(clave("abandonada")); veces != 0 {
		t.Fatalf("la tarea abandonada no deberia calcularse, se calculo %d veces", veces)
	}
	if estado := pool.Estado(); estado.Abandonadas == 0 {
		t.Fatal("el pool deberia contar la tarea abandonada")
	}
}

func TestPoolEstadoReportaLaConfiguracion(t *testing.T) {
	pool := usecase.NuevoPool(nuevaCalculadoraDoble(0), usecase.ConfigPool{Trabajadores: 8, CapacidadCola: 256})
	estado := pool.Estado()
	if estado.Trabajadores != 8 {
		t.Fatalf("trabajadores=%d, se esperaba 8", estado.Trabajadores)
	}
	if estado.CapacidadCola != 256 {
		t.Fatalf("cola_max=%d, se esperaba 256", estado.CapacidadCola)
	}
}

func TestConfigPoolValidar(t *testing.T) {
	if err := (usecase.ConfigPool{Trabajadores: 1, CapacidadCola: 1}).Validar(); err != nil {
		t.Fatalf("configuracion minima valida rechazada: %v", err)
	}
	if err := (usecase.ConfigPool{Trabajadores: 0, CapacidadCola: 8}).Validar(); err == nil {
		t.Fatal("W=0 deberia rechazarse")
	}
	if err := (usecase.ConfigPool{Trabajadores: 4, CapacidadCola: 0}).Validar(); err == nil {
		t.Fatal("cola=0 deberia rechazarse")
	}
}
