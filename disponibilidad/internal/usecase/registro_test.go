package usecase

import (
	"sync"
	"testing"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
)

func replicasDePrueba() []domain.Replica {
	return []domain.Replica{
		{ID: "A", URLBase: "http://replica-a:8080"},
		{ID: "B", URLBase: "http://replica-b:8080"},
		{ID: "C", URLBase: "http://replica-c:8080"},
	}
}

// Regla k: solo tras k fallos CONSECUTIVOS la replica pasa a CAIDA.
func TestRegistroCaeTrasKFallosConsecutivos(t *testing.T) {
	reg := NuevoRegistroReplicas(replicasDePrueba(), domain.EstadoViva, 2, 2)
	ahora := time.Now()

	if c := reg.RegistrarSondeo("B", false, 0, ahora); c != nil {
		t.Fatalf("con 1 fallo (k=2) no debe haber transicion, se obtuvo %v", c.Linea())
	}
	cambio := reg.RegistrarSondeo("B", false, 0, ahora.Add(time.Second))
	if cambio == nil {
		t.Fatal("con 2 fallos consecutivos (k=2) debe marcarse CAIDA")
	}
	if cambio.Anterior != domain.EstadoViva || cambio.Nuevo != domain.EstadoCaida {
		t.Fatalf("transicion inesperada: %s", cambio.Linea())
	}
	if estado, _ := reg.EstadoDe("B"); estado != domain.EstadoCaida {
		t.Fatalf("estado de B = %s, se esperaba CAIDA", estado)
	}
	// Fallos adicionales no deben generar mas entradas de bitacora.
	if c := reg.RegistrarSondeo("B", false, 0, ahora.Add(2*time.Second)); c != nil {
		t.Fatalf("no debe repetirse la transicion CAIDA: %s", c.Linea())
	}
}

// Un exito intermedio reinicia el contador de fallos consecutivos.
func TestRegistroExitoReiniciaContadorDeFallos(t *testing.T) {
	reg := NuevoRegistroReplicas(replicasDePrueba(), domain.EstadoViva, 3, 2)
	ahora := time.Now()

	reg.RegistrarSondeo("A", false, 0, ahora)
	reg.RegistrarSondeo("A", false, 0, ahora)
	reg.RegistrarSondeo("A", true, 5*time.Millisecond, ahora) // reinicia
	reg.RegistrarSondeo("A", false, 0, ahora)
	reg.RegistrarSondeo("A", false, 0, ahora)
	if estado, _ := reg.EstadoDe("A"); estado != domain.EstadoViva {
		t.Fatalf("A deberia seguir VIVA porque el exito reinicio el conteo, estado=%s", estado)
	}
	if c := reg.RegistrarSondeo("A", false, 0, ahora); c == nil {
		t.Fatal("al completar 3 fallos consecutivos A debe caer")
	}
}

// Regla m: tras m exitos consecutivos una replica CAIDA vuelve a VIVA.
func TestRegistroRevivelTrasMExitos(t *testing.T) {
	reg := NuevoRegistroReplicas(replicasDePrueba(), domain.EstadoCaida, 2, 3)
	ahora := time.Now()

	if c := reg.RegistrarSondeo("C", true, time.Millisecond, ahora); c != nil {
		t.Fatal("con 1 exito (m=3) no debe revivir aun")
	}
	reg.RegistrarSondeo("C", true, time.Millisecond, ahora)
	cambio := reg.RegistrarSondeo("C", true, time.Millisecond, ahora)
	if cambio == nil || cambio.Nuevo != domain.EstadoViva {
		t.Fatalf("con 3 exitos consecutivos C debe volver a VIVA, se obtuvo %v", cambio)
	}
	if len(reg.Vivas()) != 1 || reg.Vivas()[0].ID != "C" {
		t.Fatalf("Vivas() deberia contener solo a C, se obtuvo %v", reg.Vivas())
	}
}

func TestRegistroInstantaneaYDetalle(t *testing.T) {
	reg := NuevoRegistroReplicas(replicasDePrueba(), domain.EstadoViva, 1, 1)
	reg.RegistrarSondeo("B", false, 0, time.Now())

	mapa := reg.Instantanea()
	if mapa["A"] != domain.EstadoViva || mapa["B"] != domain.EstadoCaida || mapa["C"] != domain.EstadoViva {
		t.Fatalf("instantanea inesperada: %v", mapa)
	}
	detalle := reg.Detalle()
	if len(detalle) != 3 || detalle[0].ID != "A" || detalle[1].ID != "B" || detalle[2].ID != "C" {
		t.Fatalf("el detalle debe conservar el orden de configuracion: %v", detalle)
	}
	if detalle[1].UltimoCambio == nil {
		t.Fatal("B deberia tener timestamp de ultimo cambio")
	}
}

func TestRegistroKyMSeNormalizanAUno(t *testing.T) {
	reg := NuevoRegistroReplicas(replicasDePrueba(), domain.EstadoViva, 0, -5)
	if reg.K() != 1 || reg.M() != 1 {
		t.Fatalf("k=%d m=%d, se esperaba 1 y 1", reg.K(), reg.M())
	}
}

// El registro es leido por el manejador de consultas mientras el monitor lo
// escribe: esta prueba corre con -race para verificar que no hay carreras.
func TestRegistroConcurrenciaSegura(t *testing.T) {
	reg := NuevoRegistroReplicas(replicasDePrueba(), domain.EstadoViva, 2, 2)
	var wg sync.WaitGroup
	fin := make(chan struct{})

	for _, id := range []string{"A", "B", "C"} {
		wg.Add(1)
		go func(replicaID string) {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-fin:
					return
				default:
					reg.RegistrarSondeo(replicaID, i%2 == 0, time.Millisecond, time.Now())
				}
			}
		}(id)
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-fin:
					return
				default:
					_ = reg.Vivas()
					_ = reg.Instantanea()
					_ = reg.Detalle()
				}
			}
		}()
	}
	time.Sleep(150 * time.Millisecond)
	close(fin)
	wg.Wait()
}
