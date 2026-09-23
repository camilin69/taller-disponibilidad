package bitacora

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
)

func cambio(replicaID string, anterior, nuevo domain.Estado) domain.CambioEstado {
	return domain.CambioEstado{
		Momento:   time.Date(2026, 9, 8, 4, 6, 24, 682000000, time.UTC),
		ReplicaID: replicaID,
		Anterior:  anterior,
		Nuevo:     nuevo,
		Motivo:    "2 fallos consecutivos de ping",
	}
}

func TestArchivoPersisteLaTransicion(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "sub", "monitor.log")
	bit, err := NuevoArchivo(ruta, false)
	if err != nil {
		t.Fatalf("no se pudo crear la bitacora: %v", err)
	}
	defer func() { _ = bit.Cerrar() }()

	if err := bit.Registrar(cambio("B", domain.EstadoViva, domain.EstadoCaida)); err != nil {
		t.Fatalf("Registrar: %v", err)
	}

	contenido, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("no se creo el archivo (ni su directorio): %v", err)
	}
	esperado := "[2026-09-08T04:06:24.682Z] REPLICA_B VIVA -> CAIDA (2 fallos consecutivos de ping)"
	if strings.TrimSpace(string(contenido)) != esperado {
		t.Fatalf("linea escrita = %q, se esperaba %q", strings.TrimSpace(string(contenido)), esperado)
	}
	if lineas := bit.Ultimas(10); len(lineas) != 1 || lineas[0] != esperado {
		t.Fatalf("Ultimas() inesperado: %v", lineas)
	}
}

// La bitacora se abre en modo append: reiniciar el dispatcher no borra la
// evidencia de la corrida anterior.
func TestArchivoNoBorraLaEvidenciaPrevia(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "monitor.log")

	primera, _ := NuevoArchivo(ruta, false)
	_ = primera.Registrar(cambio("A", domain.EstadoViva, domain.EstadoCaida))
	_ = primera.Cerrar()

	segunda, _ := NuevoArchivo(ruta, false)
	_ = segunda.Registrar(cambio("A", domain.EstadoCaida, domain.EstadoViva))
	_ = segunda.Cerrar()

	contenido, _ := os.ReadFile(ruta)
	lineas := strings.Split(strings.TrimSpace(string(contenido)), "\n")
	if len(lineas) != 2 {
		t.Fatalf("se esperaban 2 lineas acumuladas, hay %d: %v", len(lineas), lineas)
	}
}

func TestArchivoUltimasLimitaYCopia(t *testing.T) {
	bit, _ := NuevoArchivo(filepath.Join(t.TempDir(), "monitor.log"), false)
	defer func() { _ = bit.Cerrar() }()

	for i := 0; i < 5; i++ {
		_ = bit.Registrar(cambio("B", domain.EstadoViva, domain.EstadoCaida))
	}
	if n := len(bit.Ultimas(2)); n != 2 {
		t.Fatalf("Ultimas(2) devolvio %d lineas", n)
	}
	if n := len(bit.Ultimas(0)); n != 5 {
		t.Fatalf("Ultimas(0) debe devolver todas, devolvio %d", n)
	}
	// Mutar la copia devuelta no debe afectar el buffer interno.
	copia := bit.Ultimas(1)
	copia[0] = "alterada"
	if bit.Ultimas(1)[0] == "alterada" {
		t.Fatal("Ultimas() expuso el buffer interno")
	}
}

// Varias goroutines de sondeo pueden registrar transiciones a la vez.
func TestArchivoEsSeguroEnConcurrencia(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "monitor.log")
	bit, _ := NuevoArchivo(ruta, false)
	defer func() { _ = bit.Cerrar() }()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = bit.Registrar(cambio("C", domain.EstadoViva, domain.EstadoCaida))
			_ = bit.Ultimas(5)
		}()
	}
	wg.Wait()

	contenido, _ := os.ReadFile(ruta)
	if n := len(strings.Split(strings.TrimSpace(string(contenido)), "\n")); n != 20 {
		t.Fatalf("se escribieron %d lineas, se esperaban 20", n)
	}
}
