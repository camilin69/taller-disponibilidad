// Comando metricas: calcula la tabla de resultados del taller a partir de la
// evidencia cruda (CSV del cliente + bitacora del monitor + log del inyector).
// Emite Markdown listo para pegar en la documentacion.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/metricas"
)

func main() {
	csvs := flag.String("csv", "evidencia/e0_cliente.csv,evidencia/e1_run1_cliente.csv,evidencia/e1_run2_cliente.csv",
		"lista separada por comas de CSV del cliente")
	rutaBitacora := flag.String("bitacora", "evidencia/monitor.log", "bitacora del monitor")
	rutaInyector := flag.String("inyector", "evidencia/inyector.log", "log del inyector de fallas")
	ventana := flag.Duration("ventana", 10*time.Second, "ventana de analisis posterior a la inyeccion")
	salida := flag.String("salida", "", "archivo Markdown de salida (vacio = stdout)")
	flag.Parse()

	var reporte strings.Builder
	reporte.WriteString("# Resultados experimentales\n\n")
	reporte.WriteString(fmt.Sprintf("_Generado el %s_\n\n", time.Now().Format("2006-01-02 15:04:05")))

	// --- Tabla 1: metricas por corrida ---
	transiciones, errBit := metricas.LeerBitacora(*rutaBitacora)
	if errBit != nil {
		fmt.Fprintf(os.Stderr, "aviso: no se pudo leer la bitacora %s: %v\n", *rutaBitacora, errBit)
	}
	inyecciones, errIny := metricas.LeerInyecciones(*rutaInyector)
	if errIny != nil {
		fmt.Fprintf(os.Stderr, "aviso: no se pudo leer el log del inyector %s: %v\n", *rutaInyector, errIny)
	}
	detecciones := metricas.CalcularDetecciones(inyecciones, transiciones, time.Minute)

	reporte.WriteString("## Tabla comparativa\n\n")
	reporte.WriteString("| Corrida | Solicitudes | Exitosas | Fallidas | % exito | Tiempo de deteccion | Fallos en ventana post-falla |\n")
	reporte.WriteString("|---|---:|---:|---:|---:|---:|---:|\n")

	indiceDeteccion := 0
	for _, ruta := range strings.Split(*csvs, ",") {
		ruta = strings.TrimSpace(ruta)
		if ruta == "" {
			continue
		}
		res, err := metricas.LeerCSVCliente(ruta)
		if err != nil {
			fmt.Fprintf(os.Stderr, "aviso: %v\n", err)
			continue
		}
		etiqueta := res.Experimento
		if etiqueta == "" {
			etiqueta = ruta
		}

		deteccion := "n/a (sin falla)"
		fallosVentana := "n/a"
		if perteneceAExperimentoConFalla(etiqueta) && indiceDeteccion < len(detecciones) {
			d := detecciones[indiceDeteccion]
			deteccion = fmt.Sprintf("%.3f s", d.Tiempo.Seconds())
			fallosVentana = fmt.Sprintf("%d", metricas.FallosEnVentana(res, d.Inyeccion.Momento, *ventana))
			indiceDeteccion++
		}
		reporte.WriteString(fmt.Sprintf("| %s (`%s`) | %d | %d | %d | %.2f%% | %s | %s |\n",
			etiqueta, ruta, res.Total, res.Exitosas, res.Fallidas, res.PorcentajeOK, deteccion, fallosVentana))
	}

	// --- Detalle por corrida ---
	reporte.WriteString("\n## Detalle por corrida\n")
	for _, ruta := range strings.Split(*csvs, ",") {
		ruta = strings.TrimSpace(ruta)
		if ruta == "" {
			continue
		}
		res, err := metricas.LeerCSVCliente(ruta)
		if err != nil {
			continue
		}
		reporte.WriteString(fmt.Sprintf("\n### %s (`%s`)\n\n", res.Experimento, ruta))
		reporte.WriteString(fmt.Sprintf("- Ventana: %s -> %s\n",
			res.Inicio.Format("15:04:05.000"), res.Fin.Format("15:04:05.000")))
		reporte.WriteString(fmt.Sprintf("- Latencia p50 = %.2f ms, p95 = %.2f ms\n", res.LatenciaP50, res.LatenciaP95))
		ids := make([]string, 0, len(res.PorReplica))
		for id := range res.PorReplica {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		reporte.WriteString("- Reparto de respuestas por replica: ")
		partes := make([]string, 0, len(ids))
		for _, id := range ids {
			partes = append(partes, fmt.Sprintf("%s=%d (%.1f%%)", id, res.PorReplica[id],
				100*float64(res.PorReplica[id])/float64(res.Exitosas)))
		}
		reporte.WriteString(strings.Join(partes, ", ") + "\n")
		if len(res.Fallos) > 0 {
			reporte.WriteString(fmt.Sprintf("- Solicitudes fallidas (%d):\n", len(res.Fallos)))
			for i, f := range res.Fallos {
				if i >= 10 {
					reporte.WriteString(fmt.Sprintf("  - ... y %d mas\n", len(res.Fallos)-10))
					break
				}
				reporte.WriteString(fmt.Sprintf("  - seq %s a las %s: codigo=%s %s\n",
					f.Secuencia, f.Envio.Format("15:04:05.000"), f.Codigo, f.Detalle))
			}
		} else {
			reporte.WriteString("- Solicitudes fallidas: 0\n")
		}
	}

	// --- Detecciones ---
	reporte.WriteString("\n## Tiempos de deteccion (Ping/Echo)\n\n")
	if len(detecciones) == 0 {
		reporte.WriteString("_No se encontraron parejas inyeccion/transicion._\n")
	} else {
		reporte.WriteString("| # | Inyeccion (t0) | Transicion en bitacora (t1) | Replica | Deteccion (t1-t0) |\n")
		reporte.WriteString("|---|---|---|---|---:|\n")
		for i, d := range detecciones {
			reporte.WriteString(fmt.Sprintf("| %d | %s | %s | %s | **%.3f s** |\n", i+1,
				d.Inyeccion.Momento.Format("15:04:05.000"),
				d.Transicion.Momento.Format("15:04:05.000"),
				d.Transicion.ReplicaID, d.Tiempo.Seconds()))
		}
	}

	reporte.WriteString("\n## Bitacora del monitor (transiciones)\n\n```\n")
	for _, tr := range transiciones {
		reporte.WriteString(tr.Linea + "\n")
	}
	reporte.WriteString("```\n")

	texto := reporte.String()
	if *salida == "" {
		fmt.Print(texto)
		return
	}
	if err := os.WriteFile(*salida, []byte(texto), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "no se pudo escribir %s: %v\n", *salida, err)
		os.Exit(1)
	}
	fmt.Printf("reporte escrito en %s\n", *salida)
}

// perteneceAExperimentoConFalla indica si la corrida incluyo inyeccion (E1...).
func perteneceAExperimentoConFalla(etiqueta string) bool {
	return strings.Contains(strings.ToUpper(etiqueta), "E1")
}
