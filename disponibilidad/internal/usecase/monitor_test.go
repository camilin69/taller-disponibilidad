package usecase

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
)

// R1: tras k fallos consecutivos de ping el monitor marca CAIDA y lo escribe en
// la bitacora. Se usa T pequeno para que la prueba sea rapida; la relacion
// detectada ~ k*T es la misma que en produccion (T=1s, k=2 -> ~2s).
func TestMonitorDetectaCaidaEnTiempoEsperadoYRegistraBitacora(t *testing.T) {
	const T = 50 * time.Millisecond
	replicas := replicasDePrueba()
	reg := NuevoRegistroReplicas(replicas, domain.EstadoViva, 2, 2)
	sondeador := nuevoSondeadorFalso()
	bit := &bitacoraEnMemoria{}
	mon := NuevoMonitor(replicas, reg, sondeador, bit, RelojSistema{},
		ConfigMonitor{Intervalo: T, Timeout: 20 * time.Millisecond})

	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()
	mon.Iniciar(ctx)

	time.Sleep(2 * T) // deja que el monitor confirme el estado VIVA inicial
	inicioFalla := time.Now()
	sondeador.tumbar("B")

	if !esperarCondicion(2*time.Second, func() bool {
		e, _ := reg.EstadoDe("B")
		return e == domain.EstadoCaida
	}) {
		t.Fatal("el monitor no detecto la caida de B")
	}
	deteccion := time.Since(inicioFalla)

	// Cota superior generosa para evitar intermitencias en CI: k ciclos mas uno.
	if limite := time.Duration(reg.K()+2) * T; deteccion > limite {
		t.Fatalf("deteccion demasiado lenta: %v (limite %v)", deteccion, limite)
	}
	if e, _ := reg.EstadoDe("A"); e != domain.EstadoViva {
		t.Fatalf("A no debio verse afectada, estado=%s", e)
	}
	if bit.total() != 1 {
		t.Fatalf("se esperaba exactamente 1 entrada en bitacora, hay %d: %v", bit.total(), bit.Ultimas(0))
	}
	linea := bit.Ultimas(1)[0]
	if !strings.Contains(linea, "REPLICA_B VIVA -> CAIDA") {
		t.Fatalf("formato de bitacora inesperado: %q", linea)
	}
}

// Un ping que tarda mas que el tiempo limite t cuenta como fallo.
func TestMonitorTimeoutDeSondeoCuentaComoFallo(t *testing.T) {
	replicas := replicasDePrueba()[:1]
	reg := NuevoRegistroReplicas(replicas, domain.EstadoViva, 1, 1)
	sondeador := nuevoSondeadorFalso()
	sondeador.demoras["A"] = 200 * time.Millisecond // mucho mayor que t
	bit := &bitacoraEnMemoria{}
	mon := NuevoMonitor(replicas, reg, sondeador, bit, RelojSistema{},
		ConfigMonitor{Intervalo: time.Second, Timeout: 20 * time.Millisecond})

	cambio := mon.SondearUna(context.Background(), replicas[0])
	if cambio == nil || cambio.Nuevo != domain.EstadoCaida {
		t.Fatalf("un ping que excede t debe contar como fallo, se obtuvo %v", cambio)
	}
}

// Tras m ecos consecutivos la replica vuelve a VIVA y queda registrado.
func TestMonitorReviveReplicaTrasMExitos(t *testing.T) {
	const T = 40 * time.Millisecond
	replicas := replicasDePrueba()[:2]
	reg := NuevoRegistroReplicas(replicas, domain.EstadoViva, 2, 2)
	sondeador := nuevoSondeadorFalso()
	sondeador.tumbar("B")
	bit := &bitacoraEnMemoria{}
	mon := NuevoMonitor(replicas, reg, sondeador, bit, RelojSistema{},
		ConfigMonitor{Intervalo: T, Timeout: 20 * time.Millisecond})

	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()
	mon.Iniciar(ctx)

	if !esperarCondicion(2*time.Second, func() bool {
		e, _ := reg.EstadoDe("B")
		return e == domain.EstadoCaida
	}) {
		t.Fatal("B no fue marcada CAIDA")
	}
	sondeador.levantar("B")
	if !esperarCondicion(2*time.Second, func() bool {
		e, _ := reg.EstadoDe("B")
		return e == domain.EstadoViva
	}) {
		t.Fatal("B no volvio a VIVA tras m exitos")
	}
	lineas := bit.Ultimas(0)
	if len(lineas) != 2 || !strings.Contains(lineas[1], "REPLICA_B CAIDA -> VIVA") {
		t.Fatalf("la bitacora deberia tener la transicion de recuperacion: %v", lineas)
	}
}

// El sondeo de una replica colgada no puede frenar el sondeo de las demas.
func TestMonitorReplicaLentaNoBloqueaALasOtras(t *testing.T) {
	replicas := replicasDePrueba()
	reg := NuevoRegistroReplicas(replicas, domain.EstadoViva, 2, 2)
	sondeador := nuevoSondeadorFalso()
	sondeador.demoras["A"] = 5 * time.Second // replica colgada
	bit := &bitacoraEnMemoria{}
	mon := NuevoMonitor(replicas, reg, sondeador, bit, RelojSistema{},
		ConfigMonitor{Intervalo: 30 * time.Millisecond, Timeout: 15 * time.Millisecond})

	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()
	mon.Iniciar(ctx)

	if !esperarCondicion(2*time.Second, func() bool {
		sondeador.mu.Lock()
		defer sondeador.mu.Unlock()
		return sondeador.llamadas["C"] >= 4
	}) {
		t.Fatal("C dejo de sondearse mientras A estaba colgada")
	}
	if e, _ := reg.EstadoDe("C"); e != domain.EstadoViva {
		t.Fatalf("C debe seguir VIVA, estado=%s", e)
	}
	if e, _ := reg.EstadoDe("A"); e != domain.EstadoCaida {
		t.Fatalf("A (colgada) debe terminar CAIDA, estado=%s", e)
	}
}

// esperarCondicion sondea la condicion hasta que se cumpla o venza el plazo.
func esperarCondicion(limite time.Duration, cond func() bool) bool {
	fin := time.Now().Add(limite)
	for time.Now().Before(fin) {
		if cond() {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return cond()
}
