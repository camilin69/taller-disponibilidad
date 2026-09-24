// Package usecase contiene los casos de uso (capa de aplicacion) del taller de
// desempeno: el pool de trabajadores que introduce concurrencia (R1) y la
// consulta del resumen que consulta el cache antes de calcular (R2).
// Solo depende de la capa domain y de los puertos declarados en este archivo;
// los detalles (HTTP, el costo real del calculo, relojes) viven en adapter/ e
// infra/.
package usecase

import (
	"context"
	"time"

	"github.com/camilin69/taller-desempeno/internal/domain"
)

// Calculadora es el puerto de salida del pool: ejecuta el calculo costoso del
// resumen mensual. El adaptador concreto decide como cuesta (recorrer los
// viajes reales mas el costo de leer el historial).
type Calculadora interface {
	Calcular(ctx context.Context, clave domain.ClaveResumen) (domain.Resumen, error)
}

// Cache es el puerto de salida de la tactica "Mantener Multiples Copias de
// Datos" (R2): guarda el resultado de un calculo ya realizado para que una
// solicitud repetida se responda sin volver a ejecutarlo.
type Cache interface {
	Obtener(clave domain.ClaveResumen) (domain.Resumen, bool)
	Guardar(clave domain.ClaveResumen, resumen domain.Resumen)
	Entradas() int
	// Activa distingue el cache real del cache apagado que se usa en E0 y E1.
	Activa() bool
}

// Reloj abstrae el tiempo para poder inyectar relojes deterministas en pruebas.
type Reloj interface {
	Ahora() time.Time
}

// RelojSistema es la implementacion real de Reloj.
type RelojSistema struct{}

// Ahora devuelve la hora actual del sistema.
func (RelojSistema) Ahora() time.Time { return time.Now() }
