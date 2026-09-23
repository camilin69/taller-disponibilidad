// Package domain contiene las entidades y reglas de negocio puras del sistema
// RECAUDO-T. Esta capa es el nucleo de la Clean Architecture: no importa nada
// de infraestructura (HTTP, archivos, Docker, configuracion); solo stdlib basica.
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Estado representa el estado de salud de una replica segun el monitor Ping/Echo.
type Estado string

const (
	// EstadoViva indica que la replica respondio al eco dentro del tiempo limite.
	EstadoViva Estado = "VIVA"
	// EstadoCaida indica que la replica acumulo k fallos consecutivos de sondeo.
	EstadoCaida Estado = "CAIDA"
)

// ErrEstadoInvalido se devuelve al construir un Estado desde texto no reconocido.
var ErrEstadoInvalido = errors.New("estado invalido: solo se admite VIVA o CAIDA")

// ParsearEstado convierte texto ("VIVA", "caida", "CAÍDA") en un Estado valido.
func ParsearEstado(texto string) (Estado, error) {
	normalizado := strings.ToUpper(strings.TrimSpace(texto))
	normalizado = strings.ReplaceAll(normalizado, "Í", "I")
	switch normalizado {
	case string(EstadoViva):
		return EstadoViva, nil
	case string(EstadoCaida):
		return EstadoCaida, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrEstadoInvalido, texto)
	}
}

// Replica es la entidad que identifica a un proceso servidor de saldo.
// El identificador es unico y la URL base es donde escucha su API HTTP.
type Replica struct {
	ID      string // identificador unico, p.ej. "A"
	URLBase string // p.ej. "http://replica-a:8080"
}

// Valida comprueba las invariantes de la entidad.
func (r Replica) Valida() error {
	if strings.TrimSpace(r.ID) == "" {
		return errors.New("la replica requiere un identificador no vacio")
	}
	if strings.TrimSpace(r.URLBase) == "" {
		return fmt.Errorf("la replica %s requiere una URL base", r.ID)
	}
	if !strings.HasPrefix(r.URLBase, "http://") && !strings.HasPrefix(r.URLBase, "https://") {
		return fmt.Errorf("la URL base de la replica %s debe iniciar con http:// o https://", r.ID)
	}
	return nil
}

// URLPing devuelve el endpoint de eco (Ping/Echo) de la replica.
func (r Replica) URLPing() string { return strings.TrimRight(r.URLBase, "/") + "/ping" }

// URLSaldo devuelve el endpoint de consulta de saldo para una tarjeta.
func (r Replica) URLSaldo(idTarjeta string) string {
	return strings.TrimRight(r.URLBase, "/") + "/saldo/" + idTarjeta
}

// CambioEstado es el evento de dominio que se escribe en la bitacora cuando el
// monitor confirma una transicion de salud de una replica.
type CambioEstado struct {
	Momento   time.Time
	ReplicaID string
	Anterior  Estado
	Nuevo     Estado
	Motivo    string // p.ej. "2 fallos consecutivos de ping"
}

// Linea rinde el cambio con el formato exigido por el taller:
// [timestamp] REPLICA_B VIVA -> CAIDA (motivo).
func (c CambioEstado) Linea() string {
	linea := fmt.Sprintf("[%s] REPLICA_%s %s -> %s",
		c.Momento.UTC().Format(FormatoTimestamp), c.ReplicaID, c.Anterior, c.Nuevo)
	if c.Motivo != "" {
		linea += " (" + c.Motivo + ")"
	}
	return linea
}

// FormatoTimestamp es el formato unico de tiempo usado en bitacora, CSV y logs
// del inyector, con milisegundos, para poder restar timestamps entre archivos.
const FormatoTimestamp = "2006-01-02T15:04:05.000Z07:00"
