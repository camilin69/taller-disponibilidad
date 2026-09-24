// Package cache implementa la tactica "Mantener Multiples Copias de Datos"
// (R2) como un almacen en memoria del proceso. El enunciado no pide base de
// datos externa ni expiracion durante los experimentos.
package cache

import (
	"sync"

	"github.com/camilin69/taller-desempeno/internal/domain"
)

// Memoria es un cache en memoria protegido por un RWMutex.
//
// Se usa RWMutex y no Mutex porque el patron de acceso es muy asimetrico: con
// 10 tarjetas y cientos de solicitudes, casi todos los accesos son lecturas.
// Un Mutex normal las serializaria y convertiria al cache en el nuevo cuello
// de botella, justo lo contrario de lo que busca la tactica.
type Memoria struct {
	mu       sync.RWMutex
	entradas map[domain.ClaveResumen]domain.Resumen
}

// NuevaMemoria construye un cache vacio.
func NuevaMemoria() *Memoria {
	return &Memoria{entradas: make(map[domain.ClaveResumen]domain.Resumen)}
}

// Obtener devuelve el resumen guardado para la clave, si existe.
func (m *Memoria) Obtener(clave domain.ClaveResumen) (domain.Resumen, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	resumen, ok := m.entradas[clave]
	return resumen, ok
}

// Guardar almacena el resultado de un calculo ya realizado.
func (m *Memoria) Guardar(clave domain.ClaveResumen, resumen domain.Resumen) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entradas[clave] = resumen
}

// Entradas informa cuantos resumenes hay guardados (lo publica GET /config).
func (m *Memoria) Entradas() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entradas)
}

// Activa indica que este es el cache real.
func (m *Memoria) Activa() bool { return true }

// Desactivada es el cache apagado que usan E0 y E1: nunca acierta y nunca
// guarda, de modo que toda solicitud ejecuta el calculo costoso. Existe para
// que apagar la tactica sea cambiar una variable de entorno y no tocar el
// codigo del caso de uso.
type Desactivada struct{}

// Obtener nunca acierta.
func (Desactivada) Obtener(domain.ClaveResumen) (domain.Resumen, bool) {
	return domain.Resumen{}, false
}

// Guardar descarta el resultado.
func (Desactivada) Guardar(domain.ClaveResumen, domain.Resumen) {}

// Entradas siempre es 0.
func (Desactivada) Entradas() int { return 0 }

// Activa indica que la tactica R2 esta apagada.
func (Desactivada) Activa() bool { return false }
