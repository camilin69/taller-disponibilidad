// Package bitacora implementa el puerto usecase.Bitacora escribiendo los
// cambios de estado en un archivo persistente (monitor.log) y en stdout.
package bitacora

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
)

// maxEnMemoria es el tamano del buffer circular que alimenta GET /bitacora.
const maxEnMemoria = 500

// Archivo escribe la bitacora en disco y conserva las ultimas lineas en
// memoria para exponerlas por HTTP a la interfaz React.
type Archivo struct {
	mu        sync.Mutex
	archivo   *os.File
	recientes []string
	aStdout   bool
}

// NuevoArchivo abre (o crea) el archivo de bitacora en modo append.
func NuevoArchivo(ruta string, aStdout bool) (*Archivo, error) {
	if dir := filepath.Dir(ruta); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("no se pudo crear el directorio de bitacora %s: %w", dir, err)
		}
	}
	f, err := os.OpenFile(ruta, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir la bitacora %s: %w", ruta, err)
	}
	return &Archivo{archivo: f, aStdout: aStdout}, nil
}

// Registrar persiste un cambio de estado. Es seguro para uso concurrente desde
// las goroutines de sondeo de varias replicas.
func (a *Archivo) Registrar(c domain.CambioEstado) error {
	linea := c.Linea()

	a.mu.Lock()
	defer a.mu.Unlock()

	a.recientes = append(a.recientes, linea)
	if len(a.recientes) > maxEnMemoria {
		a.recientes = a.recientes[len(a.recientes)-maxEnMemoria:]
	}
	if a.aStdout {
		fmt.Println(linea)
	}
	if a.archivo == nil {
		return nil
	}
	if _, err := a.archivo.WriteString(linea + "\n"); err != nil {
		return fmt.Errorf("no se pudo escribir en la bitacora: %w", err)
	}
	// Sincroniza a disco: la evidencia del experimento no puede quedarse en el
	// buffer del sistema operativo si el contenedor termina abruptamente.
	return a.archivo.Sync()
}

// Ultimas devuelve las ultimas n lineas registradas (n<=0 devuelve todas las
// que haya en memoria), de la mas antigua a la mas reciente.
func (a *Archivo) Ultimas(n int) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if n <= 0 || n > len(a.recientes) {
		n = len(a.recientes)
	}
	copia := make([]string, n)
	copy(copia, a.recientes[len(a.recientes)-n:])
	return copia
}

// Cerrar libera el archivo.
func (a *Archivo) Cerrar() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.archivo == nil {
		return nil
	}
	err := a.archivo.Close()
	a.archivo = nil
	return err
}
