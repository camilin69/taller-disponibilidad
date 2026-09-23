package metricas

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
)

// escribir crea un archivo temporal con el contenido dado.
func escribir(t *testing.T, nombre, contenido string) string {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), nombre)
	if err := os.WriteFile(ruta, []byte(contenido), 0o644); err != nil {
		t.Fatalf("no se pudo escribir %s: %v", ruta, err)
	}
	return ruta
}

func TestLeerCSVCliente(t *testing.T) {
	ruta := escribir(t, "cliente.csv", `secuencia,timestamp_envio,timestamp_respuesta,exito,replica_id,latencia_ms,codigo_http,segundo_relativo,experimento,error
1,2026-09-08T04:06:21.000Z,2026-09-08T04:06:21.010Z,1,A,10.000,200,1.000,E1,
2,2026-09-08T04:06:22.000Z,2026-09-08T04:06:22.020Z,1,B,20.000,200,2.000,E1,
3,2026-09-08T04:06:23.000Z,2026-09-08T04:06:23.500Z,0,,500.000,503,3.000,E1,sin replicas VIVA
4,2026-09-08T04:06:24.000Z,2026-09-08T04:06:24.030Z,1,A,30.000,200,4.000,E1,
`)

	res, err := LeerCSVCliente(ruta)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if res.Total != 4 || res.Exitosas != 3 || res.Fallidas != 1 {
		t.Fatalf("conteos inesperados: %+v", res)
	}
	if res.PorcentajeOK != 75 {
		t.Fatalf("%% de exito = %.2f, se esperaba 75", res.PorcentajeOK)
	}
	if res.Experimento != "E1" {
		t.Fatalf("experimento = %q", res.Experimento)
	}
	if res.PorReplica["A"] != 2 || res.PorReplica["B"] != 1 {
		t.Fatalf("reparto por replica inesperado: %v", res.PorReplica)
	}
	if len(res.Fallos) != 1 || res.Fallos[0].Codigo != "503" {
		t.Fatalf("no se registro la fila fallida: %+v", res.Fallos)
	}
	if res.LatenciaP50 <= 0 || res.LatenciaP95 <= 0 {
		t.Fatalf("latencias no calculadas: p50=%.2f p95=%.2f", res.LatenciaP50, res.LatenciaP95)
	}
}

func TestLeerCSVClienteRechazaColumnasFaltantes(t *testing.T) {
	ruta := escribir(t, "malo.csv", "a,b\n1,2\n")
	if _, err := LeerCSVCliente(ruta); err == nil {
		t.Fatal("un CSV sin las columnas requeridas debe fallar")
	}
}

func TestLeerBitacora(t *testing.T) {
	ruta := escribir(t, "monitor.log", `[2026-09-08T04:06:24.682Z] REPLICA_B VIVA -> CAIDA (2 fallos consecutivos de ping)
linea que no es una transicion
[2026-09-08T04:06:38.383Z] REPLICA_B CAIDA -> VIVA (2 exitos consecutivos de ping)
`)

	transiciones, err := LeerBitacora(ruta)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(transiciones) != 2 {
		t.Fatalf("se esperaban 2 transiciones, hay %d", len(transiciones))
	}
	if transiciones[0].ReplicaID != "B" || transiciones[0].Nuevo != domain.EstadoCaida {
		t.Fatalf("primera transicion inesperada: %+v", transiciones[0])
	}
	if transiciones[1].Nuevo != domain.EstadoViva {
		t.Fatalf("segunda transicion inesperada: %+v", transiciones[1])
	}
}

// La bitácora también puede venir con la flecha unicode del enunciado.
func TestLeerBitacoraAceptaFlechaUnicode(t *testing.T) {
	ruta := escribir(t, "monitor.log", "[2026-09-08T04:06:24.682Z] REPLICA_A VIVA → CAIDA\n")
	transiciones, err := LeerBitacora(ruta)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(transiciones) != 1 || transiciones[0].ReplicaID != "A" {
		t.Fatalf("no se interpreto la flecha unicode: %+v", transiciones)
	}
}

func TestLeerInyecciones(t *testing.T) {
	ruta := escribir(t, "inyector.log", `[2026-09-08T04:06:22.402Z] INYECTOR objetivo=recaudo-replica-b modo=docker comando=kill
[2026-09-08T04:06:22.813Z] INYECTOR resultado=OK duracion_ms=411
`)

	inyecciones, err := LeerInyecciones(ruta)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(inyecciones) != 1 {
		t.Fatalf("se esperaba 1 inyeccion, hay %d", len(inyecciones))
	}
	if inyecciones[0].Objetivo != "recaudo-replica-b" || inyecciones[0].Modo != "docker" {
		t.Fatalf("inyeccion inesperada: %+v", inyecciones[0])
	}
}

// El tiempo de deteccion es t_bitacora - t_inyector para la replica objetivo.
func TestCalcularDetecciones(t *testing.T) {
	inyector := escribir(t, "inyector.log", `[2026-09-08T04:06:22.402Z] INYECTOR objetivo=recaudo-replica-b modo=docker comando=kill
[2026-09-08T04:07:02.278Z] INYECTOR objetivo=recaudo-replica-b modo=docker comando=kill
`)
	bitacora := escribir(t, "monitor.log", `[2026-09-08T04:06:24.682Z] REPLICA_B VIVA -> CAIDA (2 fallos consecutivos de ping)
[2026-09-08T04:06:38.383Z] REPLICA_B CAIDA -> VIVA (2 exitos consecutivos de ping)
[2026-09-08T04:07:03.678Z] REPLICA_B VIVA -> CAIDA (2 fallos consecutivos de ping)
`)

	inyecciones, _ := LeerInyecciones(inyector)
	transiciones, _ := LeerBitacora(bitacora)
	detecciones := CalcularDetecciones(inyecciones, transiciones, time.Minute)

	if len(detecciones) != 2 {
		t.Fatalf("se esperaban 2 detecciones, hay %d", len(detecciones))
	}
	if d := detecciones[0].Tiempo; d != 2280*time.Millisecond {
		t.Fatalf("deteccion 1 = %v, se esperaba 2.280 s", d)
	}
	if d := detecciones[1].Tiempo; d != 1400*time.Millisecond {
		t.Fatalf("deteccion 2 = %v, se esperaba 1.400 s", d)
	}
	// Una transicion no puede emparejarse con dos inyecciones distintas.
	if detecciones[0].Transicion.Momento.Equal(detecciones[1].Transicion.Momento) {
		t.Fatal("la misma transicion se uso dos veces")
	}
}

// Una transicion de OTRA replica no cuenta como deteccion de esta inyeccion.
func TestCalcularDeteccionesIgnoraOtraReplica(t *testing.T) {
	inyector := escribir(t, "inyector.log",
		"[2026-09-08T04:06:22.402Z] INYECTOR objetivo=recaudo-replica-b modo=docker comando=kill\n")
	bitacora := escribir(t, "monitor.log",
		"[2026-09-08T04:06:24.682Z] REPLICA_C VIVA -> CAIDA (2 fallos consecutivos de ping)\n")

	inyecciones, _ := LeerInyecciones(inyector)
	transiciones, _ := LeerBitacora(bitacora)
	if d := CalcularDetecciones(inyecciones, transiciones, time.Minute); len(d) != 0 {
		t.Fatalf("no debio emparejarse con otra replica: %+v", d)
	}
}

func TestFallosEnVentana(t *testing.T) {
	ruta := escribir(t, "cliente.csv", `secuencia,timestamp_envio,timestamp_respuesta,exito,replica_id,latencia_ms,codigo_http,segundo_relativo,experimento,error
1,2026-09-08T04:06:20.000Z,2026-09-08T04:06:20.500Z,0,,500.000,503,1.000,E1,antes de la ventana
2,2026-09-08T04:06:23.000Z,2026-09-08T04:06:23.500Z,0,,500.000,503,4.000,E1,dentro de la ventana
3,2026-09-08T04:06:40.000Z,2026-09-08T04:06:40.500Z,0,,500.000,503,21.000,E1,despues de la ventana
`)
	res, err := LeerCSVCliente(ruta)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	inicio, _ := time.Parse(domain.FormatoTimestamp, "2026-09-08T04:06:22.402Z")
	if n := FallosEnVentana(res, inicio, 10*time.Second); n != 1 {
		t.Fatalf("fallos en ventana = %d, se esperaba 1", n)
	}
}
