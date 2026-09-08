package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
)

// R2: se responde con la PRIMERA respuesta valida; la replica lenta no retrasa
// al cliente.
func TestConsultaDevuelveLaPrimeraRespuestaValida(t *testing.T) {
	reg := NuevoRegistroReplicas(replicasDePrueba(), domain.EstadoViva, 2, 2)
	pas := nuevaPasarelaFalsa()
	pas.demoras["A"] = 400 * time.Millisecond
	pas.demoras["B"] = 5 * time.Millisecond
	pas.demoras["C"] = 300 * time.Millisecond
	caso := NuevoConsultarSaldo(reg, pas, time.Second, RelojSistema{})

	inicio := time.Now()
	res, err := caso.Ejecutar(context.Background(), "1234")
	transcurrido := time.Since(inicio)

	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if res.Saldo.ReplicaID != "B" {
		t.Fatalf("gano %s, se esperaba la replica mas rapida (B)", res.Saldo.ReplicaID)
	}
	if transcurrido > 150*time.Millisecond {
		t.Fatalf("la consulta espero a las replicas lentas: %v", transcurrido)
	}
	if len(res.ReplicasUsadas) != 3 {
		t.Fatalf("la consulta debio enviarse a las 3 replicas VIVA: %v", res.ReplicasUsadas)
	}
}

// Una replica CAIDA no recibe trafico: el fan-out usa solo las VIVA.
func TestConsultaNoEnviaAReplicasCaidas(t *testing.T) {
	reg := NuevoRegistroReplicas(replicasDePrueba(), domain.EstadoViva, 1, 1)
	reg.RegistrarSondeo("B", false, 0, time.Now()) // B queda CAIDA
	pas := nuevaPasarelaFalsa()
	caso := NuevoConsultarSaldo(reg, pas, time.Second, RelojSistema{})

	res, err := caso.Ejecutar(context.Background(), "1234")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if pas.vecesLlamada("B") != 0 {
		t.Fatal("no debe consultarse una replica marcada CAIDA")
	}
	if res.Saldo.ReplicaID == "B" {
		t.Fatal("una replica CAIDA no puede ganar la carrera")
	}
}

// Aunque una replica este caida, el cliente sigue recibiendo saldo: esta es la
// evidencia de que la redundancia activa protege al cliente (E1).
func TestConsultaSobreviveACaidaDeUnaReplica(t *testing.T) {
	reg := NuevoRegistroReplicas(replicasDePrueba(), domain.EstadoViva, 2, 2)
	pas := nuevaPasarelaFalsa()
	// A "crashea" pero el monitor aun no lo sabe: sigue marcada VIVA.
	pas.errores["A"] = errors.New("connection refused")
	caso := NuevoConsultarSaldo(reg, pas, time.Second, RelojSistema{})

	for i := 0; i < 20; i++ {
		res, err := caso.Ejecutar(context.Background(), "1234")
		if err != nil {
			t.Fatalf("el cliente percibio un error en la iteracion %d: %v", i, err)
		}
		if res.Saldo.ReplicaID == "A" {
			t.Fatal("la replica caida no puede responder")
		}
	}
	est := caso.Estadisticas.Instantanea()
	if est.Exitosas != 20 || est.Fallidas != 0 {
		t.Fatalf("estadisticas inesperadas: %+v", est)
	}
}

func TestConsultaSinReplicasVivas(t *testing.T) {
	reg := NuevoRegistroReplicas(replicasDePrueba(), domain.EstadoCaida, 1, 1)
	caso := NuevoConsultarSaldo(reg, nuevaPasarelaFalsa(), time.Second, RelojSistema{})

	if _, err := caso.Ejecutar(context.Background(), "1234"); !errors.Is(err, domain.ErrSinReplicasVivas) {
		t.Fatalf("se esperaba ErrSinReplicasVivas, se obtuvo %v", err)
	}
	if est := caso.Estadisticas.Instantanea(); est.Fallidas != 1 {
		t.Fatalf("la consulta fallida debe contabilizarse: %+v", est)
	}
}

func TestConsultaTodasLasReplicasFallan(t *testing.T) {
	reg := NuevoRegistroReplicas(replicasDePrueba(), domain.EstadoViva, 1, 1)
	pas := nuevaPasarelaFalsa()
	for _, id := range []string{"A", "B", "C"} {
		pas.errores[id] = errors.New("connection refused")
	}
	caso := NuevoConsultarSaldo(reg, pas, time.Second, RelojSistema{})

	if _, err := caso.Ejecutar(context.Background(), "1234"); !errors.Is(err, domain.ErrTodasFallaron) {
		t.Fatalf("se esperaba ErrTodasFallaron, se obtuvo %v", err)
	}
}

// Una respuesta 200 pero sin saldo valido no gana la carrera.
func TestConsultaDescartaRespuestasInvalidas(t *testing.T) {
	reg := NuevoRegistroReplicas(replicasDePrueba()[:2], domain.EstadoViva, 1, 1)
	pas := nuevaPasarelaFalsa()
	pas.invalida["A"] = true
	pas.demoras["B"] = 10 * time.Millisecond
	caso := NuevoConsultarSaldo(reg, pas, time.Second, RelojSistema{})

	res, err := caso.Ejecutar(context.Background(), "1234")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if res.Saldo.ReplicaID != "B" {
		t.Fatalf("gano %s, la respuesta invalida de A debio descartarse", res.Saldo.ReplicaID)
	}
}

func TestConsultaSeAgotaElTiempo(t *testing.T) {
	reg := NuevoRegistroReplicas(replicasDePrueba(), domain.EstadoViva, 1, 1)
	pas := nuevaPasarelaFalsa()
	for _, id := range []string{"A", "B", "C"} {
		pas.demoras[id] = time.Second
	}
	caso := NuevoConsultarSaldo(reg, pas, 50*time.Millisecond, RelojSistema{})

	inicio := time.Now()
	if _, err := caso.Ejecutar(context.Background(), "1234"); !errors.Is(err, domain.ErrTodasFallaron) {
		t.Fatalf("se esperaba ErrTodasFallaron por timeout, se obtuvo %v", err)
	}
	if time.Since(inicio) > 500*time.Millisecond {
		t.Fatal("la consulta no respeto su tiempo limite")
	}
}

func TestConsultaValidaIDTarjeta(t *testing.T) {
	reg := NuevoRegistroReplicas(replicasDePrueba(), domain.EstadoViva, 1, 1)
	caso := NuevoConsultarSaldo(reg, nuevaPasarelaFalsa(), time.Second, RelojSistema{})
	if _, err := caso.Ejecutar(context.Background(), "   "); err == nil {
		t.Fatal("un id de tarjeta vacio debe rechazarse")
	}
}

// Con latencias aleatorias distintas replicas deben ganar (evidencia de E0).
func TestConsultaAlternaGanadorasSegunLatencia(t *testing.T) {
	reg := NuevoRegistroReplicas(replicasDePrueba()[:2], domain.EstadoViva, 1, 1)
	pas := nuevaPasarelaFalsa()
	caso := NuevoConsultarSaldo(reg, pas, time.Second, RelojSistema{})

	for i := 0; i < 60; i++ {
		pas.mu.Lock()
		if i%2 == 0 {
			pas.demoras["A"], pas.demoras["B"] = 0, 20*time.Millisecond
		} else {
			pas.demoras["A"], pas.demoras["B"] = 20*time.Millisecond, 0
		}
		pas.mu.Unlock()
		if _, err := caso.Ejecutar(context.Background(), "1234"); err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
	}
	est := caso.Estadisticas.Instantanea()
	if est.Ganadoras["A"] == 0 || est.Ganadoras["B"] == 0 {
		t.Fatalf("ambas replicas debieron ganar alguna carrera: %v", est.Ganadoras)
	}
}

// El dispatcher atiende muchas consultas simultaneas sin bloquearse ni correr
// carreras de datos (ejecutar con -race).
func TestConsultaConcurrenteEsSegura(t *testing.T) {
	reg := NuevoRegistroReplicas(replicasDePrueba(), domain.EstadoViva, 2, 2)
	pas := nuevaPasarelaFalsa()
	pas.demoras["A"] = 3 * time.Millisecond
	caso := NuevoConsultarSaldo(reg, pas, time.Second, RelojSistema{})

	var wg sync.WaitGroup
	const n = 200
	errores := make(chan error, n)
	// Mientras se consulta, el monitor sigue escribiendo el registro.
	fin := make(chan struct{})
	go func() {
		for {
			select {
			case <-fin:
				return
			default:
				reg.RegistrarSondeo("C", true, time.Millisecond, time.Now())
			}
		}
	}()

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := caso.Ejecutar(context.Background(), "1234"); err != nil {
				errores <- err
			}
		}()
	}
	wg.Wait()
	close(fin)
	close(errores)
	for err := range errores {
		t.Fatalf("consulta concurrente fallida: %v", err)
	}
	if est := caso.Estadisticas.Instantanea(); est.Total != n || est.Exitosas != n {
		t.Fatalf("estadisticas inesperadas: %+v", est)
	}
}
