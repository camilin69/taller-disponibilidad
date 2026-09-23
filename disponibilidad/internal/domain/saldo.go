package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrSinReplicasVivas indica que no hay ninguna replica marcada como VIVA a la
// cual reenviar la consulta: la redundancia activa se quedo sin candidatos.
var ErrSinReplicasVivas = errors.New("no hay replicas VIVA disponibles")

// ErrTodasFallaron indica que se reenvio a las replicas VIVA pero ninguna
// entrego una respuesta valida (200 + saldo) dentro del tiempo limite.
var ErrTodasFallaron = errors.New("ninguna replica entrego una respuesta valida")

// Saldo es la respuesta de negocio de una consulta de tarjeta.
type Saldo struct {
	ReplicaID string `json:"replica_id"`
	IDTarjeta string `json:"id_tarjeta,omitempty"`
	Valor     int64  `json:"saldo"`
}

// EsValida aplica la regla del taller: una respuesta solo cuenta como valida si
// viene de una replica identificada y trae un saldo disponible (no negativo).
func (s Saldo) EsValida() bool {
	return strings.TrimSpace(s.ReplicaID) != "" && s.Valor >= 0
}

// ResultadoConsulta agrega el saldo ganador con datos de observabilidad de la
// carrera entre replicas (redundancia activa).
type ResultadoConsulta struct {
	Saldo          Saldo
	Latencia       time.Duration
	ReplicasUsadas []string // replicas VIVA a las que se envio en paralelo
}

// ValidarIDTarjeta rechaza identificadores vacios o con separadores de ruta.
func ValidarIDTarjeta(id string) error {
	limpio := strings.TrimSpace(id)
	if limpio == "" {
		return errors.New("el id de tarjeta es obligatorio")
	}
	if strings.ContainsAny(limpio, "/?#") {
		return fmt.Errorf("id de tarjeta invalido: %q", id)
	}
	return nil
}

// CalcularSaldo deriva un saldo determinista a partir del identificador de la
// tarjeta (hash FNV-1a de 32 bits). Al ser determinista, todas las replicas
// entregan el MISMO saldo para la misma tarjeta: son replicas del mismo dato,
// no servidores con datos distintos.
func CalcularSaldo(base int64, idTarjeta string) int64 {
	const offset32 = 2166136261
	const prime32 = 16777619
	h := uint32(offset32)
	for i := 0; i < len(idTarjeta); i++ {
		h ^= uint32(idTarjeta[i])
		h *= prime32
	}
	return base + int64(h%50)*100
}
