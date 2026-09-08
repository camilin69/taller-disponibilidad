// Package metricas calcula, a partir de la evidencia cruda (CSV del cliente,
// bitacora del monitor y log del inyector), las dos metricas del taller:
// tiempo de deteccion y porcentaje de solicitudes exitosas.
package metricas

import (
	"encoding/csv"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
)

// ResumenCSV son los numeros derivados del CSV del cliente.
type ResumenCSV struct {
	Archivo      string
	Experimento  string
	Total        int
	Exitosas     int
	Fallidas     int
	PorcentajeOK float64
	PorReplica   map[string]int
	Inicio       time.Time
	Fin          time.Time
	LatenciaP50  float64
	LatenciaP95  float64
	Fallos       []FilaFallida
}

// FilaFallida describe una solicitud con exito=0 (para la evidencia).
type FilaFallida struct {
	Secuencia string
	Envio     time.Time
	Codigo    string
	Detalle   string
}

// Inyeccion es un evento registrado por el inyector de fallas.
type Inyeccion struct {
	Momento  time.Time
	Objetivo string
	Modo     string
}

// Transicion es un cambio de estado leido de la bitacora del monitor.
type Transicion struct {
	Momento   time.Time
	ReplicaID string
	Anterior  domain.Estado
	Nuevo     domain.Estado
	Linea     string
}

// Deteccion empareja una inyeccion con la transicion VIVA->CAIDA que provoco.
type Deteccion struct {
	Inyeccion  Inyeccion
	Transicion Transicion
	Tiempo     time.Duration
}

var (
	// [2026-09-07T15:04:05.123Z] REPLICA_B VIVA -> CAIDA (2 fallos consecutivos)
	reTransicion = regexp.MustCompile(`^\[([^\]]+)\]\s+REPLICA_(\S+)\s+(\S+)\s*(?:->|→)\s*(\S+)`)
	// [2026-09-07T15:04:03.000Z] INYECTOR objetivo=recaudo-replica-b modo=docker ...
	reInyeccion = regexp.MustCompile(`^\[([^\]]+)\]\s+INYECTOR\s+objetivo=(\S*)\s+modo=(\S*)`)
)

// LeerCSVCliente analiza el CSV producido por el cliente.
func LeerCSVCliente(ruta string) (ResumenCSV, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return ResumenCSV{}, err
	}
	defer func() { _ = f.Close() }()

	lector := csv.NewReader(f)
	lector.FieldsPerRecord = -1
	filas, err := lector.ReadAll()
	if err != nil {
		return ResumenCSV{}, fmt.Errorf("%s: %w", ruta, err)
	}
	if len(filas) < 2 {
		return ResumenCSV{}, fmt.Errorf("%s: el CSV no tiene filas de datos", ruta)
	}

	indice := map[string]int{}
	for i, columna := range filas[0] {
		indice[strings.TrimSpace(columna)] = i
	}
	requeridas := []string{"timestamp_envio", "timestamp_respuesta", "exito", "replica_id"}
	for _, c := range requeridas {
		if _, ok := indice[c]; !ok {
			return ResumenCSV{}, fmt.Errorf("%s: falta la columna %q", ruta, c)
		}
	}

	res := ResumenCSV{Archivo: ruta, PorReplica: map[string]int{}}
	var latencias []float64
	for _, fila := range filas[1:] {
		if len(fila) < len(requeridas) {
			continue
		}
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
		if valor("exito") == "1" {
			res.Exitosas++
			res.PorReplica[valor("replica_id")]++
		} else {
			res.Fallidas++
			res.Fallos = append(res.Fallos, FilaFallida{
				Secuencia: valor("secuencia"),
				Envio:     envio,
				Codigo:    valor("codigo_http"),
				Detalle:   valor("error"),
			})
		}
		if !envio.IsZero() && !respuesta.IsZero() {
			latencias = append(latencias, float64(respuesta.Sub(envio).Microseconds())/1000.0)
		}
	}
	if res.Total > 0 {
		res.PorcentajeOK = 100 * float64(res.Exitosas) / float64(res.Total)
	}
	res.LatenciaP50 = percentil(latencias, 50)
	res.LatenciaP95 = percentil(latencias, 95)
	return res, nil
}

// LeerBitacora analiza monitor.log y devuelve las transiciones de estado.
func LeerBitacora(ruta string) ([]Transicion, error) {
	contenido, err := os.ReadFile(ruta)
	if err != nil {
		return nil, err
	}
	var transiciones []Transicion
	for _, linea := range strings.Split(string(contenido), "\n") {
		linea = strings.TrimSpace(linea)
		if linea == "" {
			continue
		}
		grupos := reTransicion.FindStringSubmatch(linea)
		if grupos == nil {
			continue
		}
		momento, err := time.Parse(domain.FormatoTimestamp, grupos[1])
		if err != nil {
			continue
		}
		anterior, err1 := domain.ParsearEstado(grupos[3])
		nuevo, err2 := domain.ParsearEstado(grupos[4])
		if err1 != nil || err2 != nil {
			continue
		}
		transiciones = append(transiciones, Transicion{
			Momento: momento, ReplicaID: grupos[2], Anterior: anterior, Nuevo: nuevo, Linea: linea,
		})
	}
	sort.Slice(transiciones, func(i, j int) bool { return transiciones[i].Momento.Before(transiciones[j].Momento) })
	return transiciones, nil
}

// LeerInyecciones analiza el log del inyector de fallas.
func LeerInyecciones(ruta string) ([]Inyeccion, error) {
	contenido, err := os.ReadFile(ruta)
	if err != nil {
		return nil, err
	}
	var inyecciones []Inyeccion
	for _, linea := range strings.Split(string(contenido), "\n") {
		grupos := reInyeccion.FindStringSubmatch(strings.TrimSpace(linea))
		if grupos == nil {
			continue
		}
		momento, err := time.Parse(domain.FormatoTimestamp, grupos[1])
		if err != nil {
			continue
		}
		inyecciones = append(inyecciones, Inyeccion{Momento: momento, Objetivo: grupos[2], Modo: grupos[3]})
	}
	sort.Slice(inyecciones, func(i, j int) bool { return inyecciones[i].Momento.Before(inyecciones[j].Momento) })
	return inyecciones, nil
}

// CalcularDetecciones empareja cada inyeccion con la primera transicion
// VIVA->CAIDA posterior: tiempo de deteccion = t_bitacora - t_inyector.
func CalcularDetecciones(inyecciones []Inyeccion, transiciones []Transicion, ventanaMax time.Duration) []Deteccion {
	if ventanaMax <= 0 {
		ventanaMax = 60 * time.Second
	}
	usadas := map[int]bool{}
	var detecciones []Deteccion
	for _, iny := range inyecciones {
		for i, tr := range transiciones {
			if usadas[i] || tr.Nuevo != domain.EstadoCaida {
				continue
			}
			if tr.Momento.Before(iny.Momento) {
				continue
			}
			if tr.Momento.Sub(iny.Momento) > ventanaMax {
				continue
			}
			// Si el objetivo del inyector nombra la replica, deben coincidir.
			if !objetivoCoincide(iny.Objetivo, tr.ReplicaID) {
				continue
			}
			usadas[i] = true
			detecciones = append(detecciones, Deteccion{Inyeccion: iny, Transicion: tr, Tiempo: tr.Momento.Sub(iny.Momento)})
			break
		}
	}
	return detecciones
}

// objetivoCoincide acepta "recaudo-replica-b", "replica-b", "B" o vacio.
func objetivoCoincide(objetivo, replicaID string) bool {
	if strings.TrimSpace(objetivo) == "" {
		return true
	}
	obj := strings.ToUpper(objetivo)
	id := strings.ToUpper(replicaID)
	return strings.HasSuffix(obj, "-"+id) || obj == id || strings.Contains(obj, "-"+id+"-")
}

// FallosEnVentana cuenta las solicitudes fallidas entre inicio e inicio+duracion.
func FallosEnVentana(res ResumenCSV, inicio time.Time, duracion time.Duration) int {
	fin := inicio.Add(duracion)
	n := 0
	for _, f := range res.Fallos {
		if !f.Envio.Before(inicio) && f.Envio.Before(fin) {
			n++
		}
	}
	return n
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
