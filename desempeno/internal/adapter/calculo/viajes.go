// Package calculo implementa el calculo costoso del resumen mensual: el
// recorrido real de los viajes del mes mas el costo de leerlos del historial.
package calculo

import (
	"context"
	"time"

	"github.com/camilin69/taller-desempeno/internal/domain"
)

// Viajes es la Calculadora real del servicio.
//
// El costo tiene dos partes, y las dos son deliberadas:
//
//   - Trabajo real: se recorre viaje por viaje y se acumula el gasto
//     (domain.SumarViajes). Es el calculo de verdad; su resultado depende de
//     los datos y es lo que el cache guarda.
//   - Costo de lectura: recorrer el historial de un mes en el servicio real
//     significa leer muchos registros de la base de datos, y eso es espera de
//     E/S, no CPU. Se modela como una espera hasta completar CostoLectura.
//
// Al rellenar hasta un total FIJO, toda solicitud calculada tarda lo mismo
// (restriccion 1 del enunciado: duracion fija y comparable entre corridas), de
// modo que las diferencias entre E0, E1 y E2 vienen de las tacticas y no del
// ruido del calculo.
type Viajes struct {
	// CostoLectura es la duracion total que debe tomar un calculo completo.
	CostoLectura time.Duration
}

// NuevaViajes construye la calculadora con el costo fijo indicado.
func NuevaViajes(costoLectura time.Duration) Viajes {
	return Viajes{CostoLectura: costoLectura}
}

// Calcular produce el resumen mensual de una tarjeta.
func (v Viajes) Calcular(ctx context.Context, clave domain.ClaveResumen) (domain.Resumen, error) {
	inicio := time.Now()

	gasto := domain.SumarViajes(clave.IDTarjeta, clave.Viajes)

	// Rellenar hasta el costo fijo. Se respeta el contexto para no seguir
	// ocupando al trabajador si quien pidio el resumen ya se rindio.
	if restante := v.CostoLectura - time.Since(inicio); restante > 0 {
		temporizador := time.NewTimer(restante)
		defer temporizador.Stop()
		select {
		case <-temporizador.C:
		case <-ctx.Done():
			return domain.Resumen{}, ctx.Err()
		}
	}

	return domain.Resumen{
		IDTarjeta:   clave.IDTarjeta,
		TotalViajes: clave.Viajes,
		GastoTotal:  gasto,
		DesdeCache:  false,
	}, nil
}
