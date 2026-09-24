// Comando metricas: calcula la tabla de resultados del taller a partir de los
// CSV del cliente (E0, E1, E2). Emite Markdown listo para pegar en la
// documentacion. Un numero sin su CSV no cuenta como medicion, asi que cada
// fila de la tabla nombra el archivo del que proviene.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/camilin69/taller-desempeno/internal/metricas"
)

func main() {
	csvs := flag.String("csv", "evidencia/e0_cliente.csv,evidencia/e1_cliente.csv,evidencia/e2_run1_cliente.csv,evidencia/e2_run2_cliente.csv",
		"lista separada por comas de CSV del cliente")
	salida := flag.String("salida", "", "archivo Markdown de salida (vacio = stdout)")
	flag.Parse()

	var resumenes []metricas.Resumen
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
		resumenes = append(resumenes, res)
	}
	if len(resumenes) == 0 {
		fmt.Fprintln(os.Stderr, "no se pudo leer ningun CSV: ejecute primero los experimentos")
		os.Exit(1)
	}

	var reporte strings.Builder
	reporte.WriteString("# Resultados experimentales · tácticas de desempeño\n\n")
	reporte.WriteString(fmt.Sprintf("_Generado el %s_\n\n", time.Now().Format("2006-01-02 15:04:05")))

	escribirTablaPrincipal(&reporte, resumenes)
	escribirTablaCache(&reporte, resumenes)
	escribirDetalle(&reporte, resumenes)

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

// escribirTablaPrincipal emite la tabla con las cuatro medidas de la teoria.
func escribirTablaPrincipal(r *strings.Builder, resumenes []metricas.Resumen) {
	r.WriteString("## Tabla de métricas\n\n")
	r.WriteString("| Corrida | Evidencia | Solicitudes | Latencia media | Latencia p95 | Throughput | Jitter | No procesadas | % desde caché |\n")
	r.WriteString("|---|---|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, res := range resumenes {
		r.WriteString(fmt.Sprintf("| %s | `%s` | %d | %.1f ms | %.1f ms | %.2f req/s | %.1f ms | %d (%.1f %%) | %.1f %% |\n",
			etiqueta(res), res.Archivo, res.Total,
			res.LatenciaPromedio, res.LatenciaP95, res.Throughput, res.Jitter,
			res.NoProcesadas, pct(res.NoProcesadas, res.Total), res.PorcentajeCache))
	}
	r.WriteString("\n")
}

// escribirTablaCache compara la latencia de un acierto contra la de un calculo.
func escribirTablaCache(r *strings.Builder, resumenes []metricas.Resumen) {
	r.WriteString("## Efecto del caché (R2)\n\n")
	r.WriteString("| Corrida | Desde caché | Calculadas | Latencia media desde caché | Latencia media calculada |\n")
	r.WriteString("|---|---:|---:|---:|---:|\n")
	for _, res := range resumenes {
		calculadas := res.Exitosas - res.DesdeCache
		latenciaCache := "n/a"
		if res.DesdeCache > 0 {
			latenciaCache = fmt.Sprintf("%.1f ms", res.LatenciaCache)
		}
		latenciaCalculo := "n/a"
		if calculadas > 0 {
			latenciaCalculo = fmt.Sprintf("%.1f ms", res.LatenciaCalculo)
		}
		r.WriteString(fmt.Sprintf("| %s | %d | %d | %s | %s |\n",
			etiqueta(res), res.DesdeCache, calculadas, latenciaCache, latenciaCalculo))
	}
	r.WriteString("\n")
}

// escribirDetalle emite el desglose por corrida, con las solicitudes fallidas.
func escribirDetalle(r *strings.Builder, resumenes []metricas.Resumen) {
	r.WriteString("## Detalle por corrida\n")
	for _, res := range resumenes {
		r.WriteString(fmt.Sprintf("\n### %s (`%s`)\n\n", etiqueta(res), res.Archivo))
		r.WriteString(fmt.Sprintf("- Ventana: %s → %s (%.1f s)\n",
			res.Inicio.Format("15:04:05.000"), res.Fin.Format("15:04:05.000"), res.Duracion.Seconds()))
		r.WriteString(fmt.Sprintf("- Solicitudes: %d enviadas, %d exitosas, %d no procesadas\n",
			res.Total, res.Exitosas, res.NoProcesadas))
		r.WriteString(fmt.Sprintf("- Latencia: media %.1f ms · p50 %.1f ms · p95 %.1f ms · min %.1f ms · máx %.1f ms\n",
			res.LatenciaPromedio, res.LatenciaP50, res.LatenciaP95, res.LatenciaMin, res.LatenciaMax))
		r.WriteString(fmt.Sprintf("- Throughput: %.2f solicitudes exitosas por segundo\n", res.Throughput))
		r.WriteString(fmt.Sprintf("- Jitter: %.1f ms de variación media entre solicitudes consecutivas\n", res.Jitter))
		r.WriteString(fmt.Sprintf("- Desde caché: %d de %d (%.1f %%)\n",
			res.DesdeCache, res.Total, res.PorcentajeCache))

		if len(res.Fallos) == 0 {
			r.WriteString("- Solicitudes no procesadas: 0\n")
			continue
		}
		r.WriteString(fmt.Sprintf("- Solicitudes no procesadas (%d):\n", len(res.Fallos)))
		for i, f := range res.Fallos {
			if i >= 10 {
				r.WriteString(fmt.Sprintf("  - ... y %d más\n", len(res.Fallos)-10))
				break
			}
			r.WriteString(fmt.Sprintf("  - seq %s a las %s: código=%s %s\n",
				f.Secuencia, f.Envio.Format("15:04:05.000"), f.Codigo, f.Detalle))
		}
	}
	r.WriteString("\n")
}

// etiqueta prefiere el nombre del experimento y cae al archivo si falta.
func etiqueta(res metricas.Resumen) string {
	if res.Experimento != "" {
		return res.Experimento
	}
	return res.Archivo
}

func pct(parte, total int) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(parte) / float64(total)
}
