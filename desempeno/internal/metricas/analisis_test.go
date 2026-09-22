package metricas_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/camilin69/taller-desempeno/internal/metricas"
)

// escribirCSV deja un CSV temporal con el mismo encabezado que produce el
// cliente, para probar el analisis sin correr un experimento completo.
func escribirCSV(t *testing.T, filas string) string {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), "cliente.csv")
	contenido := "secuencia,timestamp_envio,timestamp_respuesta,exito,desde_cache,id_tarjeta," +
		"latencia_ms,codigo_http,segundo_relativo,experimento,error\n" + filas
	if err := os.WriteFile(ruta, []byte(contenido), 0o644); err != nil {
		t.Fatalf("no se pudo escribir el CSV de prueba: %v", err)
	}
	return ruta
}

func TestLeerCSVCalculaLasCuatroMedidas(t *testing.T) {
	// Cuatro solicitudes en 1 s: tres exitosas (100, 300 y 200 ms) y una no
	// procesada. Una de las exitosas vino del cache.
	ruta := escribirCSV(t, ""+
		"1,2026-09-22T10:00:00.000Z,2026-09-22T10:00:00.100Z,1,0,1000,100.000,200,0.000,E2,\n"+
		"2,2026-09-22T10:00:00.200Z,2026-09-22T10:00:00.500Z,1,0,1001,300.000,200,0.200,E2,\n"+
		"3,2026-09-22T10:00:00.400Z,2026-09-22T10:00:00.600Z,1,1,1000,200.000,200,0.400,E2,\n"+
		"4,2026-09-22T10:00:00.600Z,2026-09-22T10:00:01.000Z,0,0,1002,400.000,0,0.600,E2,timeout\n")

	res, err := metricas.LeerCSVCliente(ruta)
	if err != nil {
		t.Fatalf("no se pudo analizar el CSV: %v", err)
	}

	if res.Experimento != "E2" {
		t.Fatalf("experimento=%q, se esperaba \"E2\"", res.Experimento)
	}
	if res.Total != 4 {
		t.Fatalf("total=%d, se esperaban 4", res.Total)
	}
	if res.Exitosas != 3 {
		t.Fatalf("exitosas=%d, se esperaban 3", res.Exitosas)
	}
	// Cuarta medida: eventos no procesados.
	if res.NoProcesadas != 1 {
		t.Fatalf("no procesadas=%d, se esperaba 1", res.NoProcesadas)
	}

	// Latencia: (100 + 300 + 200) / 3 = 200 ms.
	if res.LatenciaPromedio != 200 {
		t.Fatalf("latencia promedio=%.1f ms, se esperaban 200", res.LatenciaPromedio)
	}

	// Throughput: 3 exitosas en la ventana 10:00:00.000 -> 10:00:01.000 = 3 req/s.
	if res.Throughput != 3 {
		t.Fatalf("throughput=%.2f req/s, se esperaban 3", res.Throughput)
	}

	// Jitter: |300-100| + |200-300| = 300, entre 2 pares = 150 ms.
	if res.Jitter != 150 {
		t.Fatalf("jitter=%.1f ms, se esperaban 150", res.Jitter)
	}

	// Cache: 1 de 4 filas = 25 %.
	if res.DesdeCache != 1 {
		t.Fatalf("desde cache=%d, se esperaba 1", res.DesdeCache)
	}
	if res.PorcentajeCache != 25 {
		t.Fatalf("porcentaje de cache=%.1f%%, se esperaba 25", res.PorcentajeCache)
	}
}

// El taller pide comparar la latencia de lo servido desde cache contra la de
// lo calculado.
func TestLeerCSVSeparaLatenciaDeCacheYDeCalculo(t *testing.T) {
	ruta := escribirCSV(t, ""+
		"1,2026-09-22T10:00:00.000Z,2026-09-22T10:00:00.200Z,1,0,1000,200.000,200,0.000,E2,\n"+
		"2,2026-09-22T10:00:00.300Z,2026-09-22T10:00:00.500Z,1,0,1001,200.000,200,0.300,E2,\n"+
		"3,2026-09-22T10:00:00.600Z,2026-09-22T10:00:00.602Z,1,1,1000,2.000,200,0.600,E2,\n"+
		"4,2026-09-22T10:00:00.700Z,2026-09-22T10:00:00.704Z,1,1,1001,4.000,200,0.700,E2,\n")

	res, err := metricas.LeerCSVCliente(ruta)
	if err != nil {
		t.Fatalf("no se pudo analizar el CSV: %v", err)
	}
	if res.LatenciaCalculo != 200 {
		t.Fatalf("latencia calculada=%.1f ms, se esperaban 200", res.LatenciaCalculo)
	}
	if res.LatenciaCache != 3 {
		t.Fatalf("latencia desde cache=%.1f ms, se esperaban 3", res.LatenciaCache)
	}
}

// Las solicitudes no procesadas no deben contaminar la latencia ni el
// throughput: se cuentan aparte, como dice el enunciado.
func TestLasNoProcesadasNoEntranEnLaLatencia(t *testing.T) {
	ruta := escribirCSV(t, ""+
		"1,2026-09-22T10:00:00.000Z,2026-09-22T10:00:00.100Z,1,0,1000,100.000,200,0.000,E0,\n"+
		"2,2026-09-22T10:00:00.100Z,2026-09-22T10:00:03.100Z,0,0,1001,3000.000,0,0.100,E0,timeout\n")

	res, err := metricas.LeerCSVCliente(ruta)
	if err != nil {
		t.Fatalf("no se pudo analizar el CSV: %v", err)
	}
	if res.LatenciaPromedio != 100 {
		t.Fatalf("latencia promedio=%.1f ms; la fila fallida no deberia contar", res.LatenciaPromedio)
	}
	if len(res.Fallos) != 1 {
		t.Fatalf("se registraron %d fallos, se esperaba 1", len(res.Fallos))
	}
	if res.Fallos[0].Detalle != "timeout" {
		t.Fatalf("detalle del fallo=%q, se esperaba \"timeout\"", res.Fallos[0].Detalle)
	}
}

func TestLeerCSVExigeLasColumnasDelContrato(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "malo.csv")
	// Sin la columna desde_cache no se puede medir el efecto del cache.
	contenido := "secuencia,timestamp_envio,timestamp_respuesta,exito,id_tarjeta\n" +
		"1,2026-09-22T10:00:00.000Z,2026-09-22T10:00:00.100Z,1,1000\n"
	if err := os.WriteFile(ruta, []byte(contenido), 0o644); err != nil {
		t.Fatalf("no se pudo escribir el CSV: %v", err)
	}

	if _, err := metricas.LeerCSVCliente(ruta); err == nil {
		t.Fatal("se esperaba un error por la columna desde_cache faltante")
	}
}

func TestLeerCSVSinFilas(t *testing.T) {
	ruta := escribirCSV(t, "")
	if _, err := metricas.LeerCSVCliente(ruta); err == nil {
		t.Fatal("un CSV sin filas de datos deberia dar error")
	}
}

func TestLeerCSVArchivoInexistente(t *testing.T) {
	if _, err := metricas.LeerCSVCliente(filepath.Join(t.TempDir(), "no-existe.csv")); err == nil {
		t.Fatal("se esperaba un error al leer un archivo inexistente")
	}
}
