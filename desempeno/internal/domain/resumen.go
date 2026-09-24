// Package domain contiene las entidades y reglas de negocio puras del resumen
// mensual de viajes de RECAUDO-T. Esta capa es el nucleo de la Clean
// Architecture: no importa nada de infraestructura (HTTP, pools, caches,
// configuracion); solo stdlib basica.
package domain

import (
	"errors"
	"fmt"
	"strings"
)

// FormatoTimestamp es el formato UTC con milisegundos que comparten el CSV del
// cliente y las trazas del servidor, de modo que sean comparables entre si.
const FormatoTimestamp = "2006-01-02T15:04:05.000Z"

// MaxViajes acota el numero de viajes que se pueden pedir en una consulta: sin
// tope, un cliente podria disparar el costo del calculo sin limite.
const MaxViajes = 10_000

// ErrColaLlena indica que la cola de trabajo alcanzo su capacidad y la
// solicitud no se pudo encolar: es un evento NO PROCESADO, la cuarta medida de
// desempeno de la teoria.
var ErrColaLlena = errors.New("la cola de trabajo esta llena")

// Resumen es la respuesta de negocio: cuanto gasto una tarjeta en el mes.
//
// DesdeCache distingue las respuestas servidas por la tactica "Mantener
// Multiples Copias de Datos" (R2) de las que si ejecutaron el calculo costoso.
type Resumen struct {
	IDTarjeta   string `json:"id_tarjeta"`
	TotalViajes int    `json:"total_viajes"`
	GastoTotal  int64  `json:"gasto_total"`
	DesdeCache  bool   `json:"desde_cache"`
}

// ClaveResumen identifica un resultado cacheable. La clave incluye el numero de
// viajes porque el resumen de los ultimos 40 viajes NO es el mismo que el de
// los ultimos 60: cachear solo por tarjeta devolveria un total equivocado.
type ClaveResumen struct {
	IDTarjeta string
	Viajes    int
}

// String da una representacion legible para logs y pruebas ("1234/40").
func (c ClaveResumen) String() string {
	return fmt.Sprintf("%s/%d", c.IDTarjeta, c.Viajes)
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

// ValidarViajes comprueba que el numero de viajes pedido sea razonable.
func ValidarViajes(viajes int) error {
	if viajes <= 0 {
		return fmt.Errorf("el numero de viajes debe ser mayor que 0, se recibio %d", viajes)
	}
	if viajes > MaxViajes {
		return fmt.Errorf("el numero de viajes no puede superar %d, se recibio %d", MaxViajes, viajes)
	}
	return nil
}

// TarifaViaje deriva el valor de un viaje concreto a partir del identificador
// de la tarjeta y del numero de viaje (hash FNV-1a de 32 bits). Al ser
// determinista, el resumen de una tarjeta siempre da el mismo gasto: por eso
// una respuesta servida desde el cache es indistinguible de una recalculada.
func TarifaViaje(idTarjeta string, n int) int64 {
	const offset32 = 2166136261
	const prime32 = 16777619
	h := uint32(offset32)
	for i := 0; i < len(idTarjeta); i++ {
		h ^= uint32(idTarjeta[i])
		h *= prime32
	}
	// Mezclar el numero de viaje byte a byte para que dos viajes consecutivos
	// de la misma tarjeta no tengan tarifas correlacionadas.
	for v := uint32(n); ; v >>= 8 {
		h ^= v & 0xFF
		h *= prime32
		if v < 256 {
			break
		}
	}
	// Tarifas entre 2.000 y 4.975 pesos, en multiplos de 25.
	return 2000 + int64(h%120)*25
}

// SumarViajes recorre los viajes del mes uno por uno y acumula el gasto. Es el
// trabajo REAL del calculo costoso: no es una formula cerrada, se recorre el
// historial completo igual que lo haria el servicio sobre su base de datos.
func SumarViajes(idTarjeta string, viajes int) int64 {
	var total int64
	for n := 1; n <= viajes; n++ {
		total += TarifaViaje(idTarjeta, n)
	}
	return total
}
