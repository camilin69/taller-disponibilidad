// Package usecase contiene los casos de uso (capa de aplicacion) del taller:
// el monitor Ping/Echo (R1) y la consulta con redundancia activa (R2).
// Solo depende de la capa domain y de los puertos declarados en este archivo;
// los detalles (HTTP, archivos, relojes reales) viven en adapter/ e infra/.
package usecase

import (
	"context"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
)

// Sondeador es el puerto de salida del monitor: envia un ping a una replica y
// espera el eco. El adaptador HTTP implementa el tiempo limite t via context.
type Sondeador interface {
	Sondear(ctx context.Context, r domain.Replica) error
}

// PasarelaSaldo es el puerto de salida de la consulta: pide el saldo a UNA
// replica concreta. La competencia en paralelo la orquesta el caso de uso.
type PasarelaSaldo interface {
	ConsultarSaldo(ctx context.Context, r domain.Replica, idTarjeta string) (domain.Saldo, error)
}

// Bitacora es el puerto de salida para persistir los cambios de estado.
type Bitacora interface {
	Registrar(c domain.CambioEstado) error
	Ultimas(n int) []string
}

// Reloj abstrae el tiempo para poder inyectar relojes deterministas en pruebas.
type Reloj interface {
	Ahora() time.Time
}

// RelojSistema es la implementacion real de Reloj.
type RelojSistema struct{}

// Ahora devuelve la hora actual del sistema.
func (RelojSistema) Ahora() time.Time { return time.Now() }
