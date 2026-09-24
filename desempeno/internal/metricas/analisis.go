// Package metricas calcula, a partir del CSV del cliente, las cuatro medidas
// de desempeno de la teoria (latencia, throughput, jitter y eventos no
// procesados) mas el porcentaje de respuestas servidas desde el cache.
package metricas

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/camilin69/taller-desempeno/internal/domain"
)

// Resumen son los numeros derivados de una corrida.
type Resumen struct {
	Archivo     string
	Experimento string

	Total        int
	Exitosas     int
	NoProcesadas int

	// --- las cuatro medidas de la teoria ---
	LatenciaPromedio float64 // ms, solo solicitudes exitosas
	LatenciaP95      float64 // ms
	Throughput       float64 // solicitudes exitosas / duracion total
	Jitter           float64 // ms, variacion media entre latencias consecutivas
	// NoProcesadas ya cuenta la cuarta medida.

	// --- efecto del cache (R2) ---
	DesdeCache      int
	PorcentajeCache float64
	LatenciaCache   float64 // ms, promedio de las respuestas servidas del cache
	LatenciaCalculo float64 // ms, promedio de las que si ejecutaron el calculo

	// --- contexto de la corrida ---
	LatenciaP50 float64
	LatenciaMin float64
	LatenciaMax float64
	Inicio      time.Time
	Fin         time.Time
	Duracion    time.Duration
	Fallos      []FilaFallida
}

// FilaFallida describe una solicitud con exito=0 (para la evidencia).
type FilaFallida struct {
	Secuencia string
	Envio     time.Time
	Codigo    string
	Detalle   string
}

// columnasRequeridas son las que el analisis necesita si o si.
var columnasRequeridas = []string{"timestamp_envio", "timestamp_respuesta", "exito", "desde_cache", "id_tarjeta"}

// LeerCSVCliente analiza el CSV producido por el cliente y calcula las metricas.
func LeerCSVCliente(ruta string) (Resumen, error) {
	filas, indice, err := abrirCSV(ruta)
	if err != nil {
		return Resumen{}, err
	}

	res := Resumen{Archivo: ruta}
	var (
		latencias        []float64 // en orden de secuencia, para el jitter
		latenciasCache   []float64
		latenciasCalculo []float64
	)

	for _, fila := range filas {
		valor := func(clave string) string {
			i, ok := indice[clave]
			if !ok || i >= len(fila) {
				return ""
			}
			return strings.TrimSpace(fila[i])
		}

		res.Total++
		envio, _ := time.Parse(domain.FormatoTimestamp, valor("timestamp_envio"))
		respuesta, _ := time.Parse(domain.FormatoTimestamp, valor("timestamp_respuesta"))

		if !envio.IsZero() {
			if res.Inicio.IsZero() || envio.Before(res.Inicio) {
				res.Inicio = envio
			}
			if respuesta.After(res.Fin) {
				res.Fin = respuesta
			}
		}
		if exp := valor("experimento"); exp != "" && res.Experimento == "" {
			res.Experimento = exp
		}

		if valor("exito") != "1" {
			// Evento no procesado: no obtuvo respuesta util dentro del tiempo
			// limite del cliente. No entra en latencia ni en throughput.
			res.NoProcesadas++
			res.Fallos = append(res.Fallos, FilaFallida{
				Secuencia: valor("secuencia"),
				Envio:     envio,
				Codigo:    valor("codigo_http"),
				Detalle:   valor("error"),
			})
			continue
		}

		res.Exitosas++
		if envio.IsZero() || respuesta.IsZero() {
			continue
		}
		latencia := float64(respuesta.Sub(envio).Microseconds()) / 1000.0
		latencias = append(latencias, latencia)

		if valor("desde_cache") == "1" {
			res.DesdeCache++
			latenciasCache = append(latenciasCache, latencia)
		} else {
			latenciasCalculo = append(latenciasCalculo, latencia)
		}
	}

	if res.Total == 0 {
		return Resumen{}, fmt.Errorf("%s: el CSV no tiene filas de datos", ruta)
	}

	// Latencia: promedio y p95 sobre las solicitudes exitosas.
	res.LatenciaPromedio = promedio(latencias)
	res.LatenciaP50 = percentil(latencias, 50)
	res.LatenciaP95 = percentil(latencias, 95)
	res.LatenciaMin, res.LatenciaMax = extremos(latencias)

	// Jitter: variacion media entre latencias consecutivas. Las filas ya vienen
	// ordenadas por secuencia, que es el orden real de emision.
	res.Jitter = jitter(latencias)

	// Throughput: solicitudes exitosas por segundo de experimento.
	res.Duracion = res.Fin.Sub(res.Inicio)
	if res.Duracion > 0 {
		res.Throughput = float64(res.Exitosas) / res.Duracion.Seconds()
	}

	// Efecto del cache.
	res.PorcentajeCache = porcentaje(res.DesdeCache, res.Total)
	res.LatenciaCache = promedio(latenciasCache)
	res.LatenciaCalculo = promedio(latenciasCalculo)

	return res, nil
}

// abrirCSV lee el archivo y valida que tenga las columnas que el analisis usa.
func abrirCSV(ruta string) ([][]string, map[string]int, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()

	lector := csv.NewReader(f)
	lector.FieldsPerRecord = -1
	filas, err := lector.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", ruta, err)
	}
	if len(filas) < 2 {
		return nil, nil, fmt.Errorf("%s: el CSV no tiene filas de datos", ruta)
	}

	indice := map[string]int{}
	for i, columna := range filas[0] {
		indice[strings.TrimSpace(columna)] = i
	}
	for _, c := range columnasRequeridas {
		if _, ok := indice[c]; !ok {
			return nil, nil, fmt.Errorf("%s: falta la columna %q", ruta, c)
		}
	}
	return filas[1:], indice, nil
}

// jitter promedia la diferencia absoluta entre la latencia de una solicitud y
// la de la inmediatamente anterior. Un jitter alto significa que el servicio
// responde de forma impredecible: unas solicitudes rapido y otras lentisimo,
// que es lo que ocurre cuando la cola crece sin control.
func jitter(latencias []float64) float64 {
	if len(latencias) < 2 {
		return 0
	}
	var suma float64
	for i := 1; i < len(latencias); i++ {
		diferencia := latencias[i] - latencias[i-1]
		if diferencia < 0 {
			diferencia = -diferencia
		}
		suma += diferencia
	}
	return suma / float64(len(latencias)-1)
}

func promedio(valores []float64) float64 {
	if len(valores) == 0 {
		return 0
	}
	var suma float64
	for _, v := range valores {
		suma += v
	}
	return suma / float64(len(valores))
}

func extremos(valores []float64) (float64, float64) {
	if len(valores) == 0 {
		return 0, 0
	}
	minimo, maximo := valores[0], valores[0]
	for _, v := range valores {
		if v < minimo {
			minimo = v
		}
		if v > maximo {
			maximo = v
		}
	}
	return minimo, maximo
}

func percentil(valores []float64, p float64) float64 {
	if len(valores) == 0 {
		return 0
	}
	ordenados := append([]float64(nil), valores...)
	sort.Float64s(ordenados)
	indice := int(p / 100 * float64(len(ordenados)-1))
	if indice < 0 {
		indice = 0
	}
	if indice >= len(ordenados) {
		indice = len(ordenados) - 1
	}
	return ordenados[indice]
}

func porcentaje(parte, total int) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(parte) / float64(total)
}
