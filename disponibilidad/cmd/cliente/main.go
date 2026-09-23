// Comando cliente: generador de carga que simula muchos validadores de bus
// consultando el saldo a ritmo constante. Por cada intento escribe una fila en
// un CSV con timestamp de envio, timestamp de respuesta, exito (0/1) y replica
// que respondio. Ese CSV es la evidencia del % de solicitudes exitosas.
package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
	"github.com/camilin69/taller-disponibilidad/internal/infra/config"
)

// intento es el registro de una solicitud al dispatcher.
type intento struct {
	secuencia       int
	envio           time.Time
	respuesta       time.Time
	exito           bool
	replicaID       string
	codigoHTTP      int
	detalleError    string
	segundoRelativo float64
}

// cuerpoSaldo es la parte de la respuesta del dispatcher que interesa al CSV.
type cuerpoSaldo struct {
	ReplicaID string `json:"replica_id"`
	Saldo     int64  `json:"saldo"`
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetPrefix("[cliente] ")

	cfg, err := config.CargarCliente()
	if err != nil {
		log.Fatalf("configuracion invalida: %v", err)
	}

	ctx, detener := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer detener()

	if err := esperarDispatcher(ctx, cfg.URLDispatcher, 60*time.Second); err != nil {
		log.Fatalf("el dispatcher no quedo disponible: %v", err)
	}
	if cfg.EsperaInicial > 0 {
		log.Printf("espera inicial de %v antes de generar carga", cfg.EsperaInicial)
		time.Sleep(cfg.EsperaInicial)
	}

	log.Printf("experimento=%s tasa=%d req/s duracion=%v tarjeta=%s destino=%s",
		cfg.Etiqueta, cfg.TasaRPS, cfg.Duracion, cfg.IDTarjeta, cfg.URLDispatcher)

	intentos := generarCarga(ctx, cfg)

	sort.Slice(intentos, func(i, j int) bool { return intentos[i].secuencia < intentos[j].secuencia })
	if err := escribirCSV(cfg.ArchivoCSV, cfg.Etiqueta, intentos); err != nil {
		log.Fatalf("no se pudo escribir el CSV: %v", err)
	}
	imprimirResumen(cfg, intentos)
}

// generarCarga dispara solicitudes a tasa constante durante la duracion fijada.
// Cada solicitud corre en su propia goroutine para que una respuesta lenta no
// altere el ritmo de emision.
func generarCarga(ctx context.Context, cfg config.Cliente) []intento {
	cliente := &http.Client{
		Timeout: cfg.Timeout,
		Transport: &http.Transport{
			MaxIdleConns:        cfg.TasaRPS * 8,
			MaxIdleConnsPerHost: cfg.TasaRPS * 4,
			MaxConnsPerHost:     cfg.TasaRPS * 8,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	intervalo := time.Second / time.Duration(cfg.TasaRPS)
	ticker := time.NewTicker(intervalo)
	defer ticker.Stop()

	inicio := time.Now()
	fin := inicio.Add(cfg.Duracion)
	url := cfg.URLDispatcher + "/saldo/" + cfg.IDTarjeta

	var (
		mu         sync.Mutex
		wg         sync.WaitGroup
		resultados []intento
		secuencia  int
	)

	for time.Now().Before(fin) {
		select {
		case <-ctx.Done():
			log.Println("carga interrumpida por senal")
			goto esperar
		case <-ticker.C:
			secuencia++
			n := secuencia
			wg.Add(1)
			go func() {
				defer wg.Done()
				res := ejecutarIntento(cliente, url, n, inicio)
				mu.Lock()
				resultados = append(resultados, res)
				mu.Unlock()
			}()
			if secuencia%(cfg.TasaRPS*10) == 0 {
				log.Printf("progreso: %d solicitudes enviadas (%.0f s)", secuencia, time.Since(inicio).Seconds())
			}
		}
	}

esperar:
	wg.Wait()
	return resultados
}

// ejecutarIntento realiza una consulta y clasifica su resultado.
func ejecutarIntento(cliente *http.Client, url string, secuencia int, inicioCarga time.Time) intento {
	envio := time.Now()
	res := intento{
		secuencia:       secuencia,
		envio:           envio,
		segundoRelativo: envio.Sub(inicioCarga).Seconds(),
	}

	respuesta, err := cliente.Get(url)
	if err != nil {
		res.respuesta = time.Now()
		res.detalleError = err.Error()
		return res
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(respuesta.Body, 8192))
		_ = respuesta.Body.Close()
	}()

	res.codigoHTTP = respuesta.StatusCode
	var cuerpo cuerpoSaldo
	errCuerpo := json.NewDecoder(io.LimitReader(respuesta.Body, 8192)).Decode(&cuerpo)
	res.respuesta = time.Now()

	if respuesta.StatusCode != http.StatusOK {
		res.detalleError = "codigo " + strconv.Itoa(respuesta.StatusCode)
		return res
	}
	if errCuerpo != nil {
		res.detalleError = "cuerpo ilegible: " + errCuerpo.Error()
		return res
	}
	res.replicaID = cuerpo.ReplicaID
	if res.replicaID == "" {
		res.replicaID = respuesta.Header.Get("X-Replica-Id")
	}
	res.exito = res.replicaID != ""
	if !res.exito {
		res.detalleError = "respuesta sin replica_id"
	}
	return res
}

// escribirCSV persiste la evidencia del experimento.
func escribirCSV(ruta, etiqueta string, intentos []intento) error {
	if dir := filepath.Dir(ruta); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.Create(ruta)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	defer w.Flush()

	encabezado := []string{"secuencia", "timestamp_envio", "timestamp_respuesta", "exito",
		"replica_id", "latencia_ms", "codigo_http", "segundo_relativo", "experimento", "error"}
	if err := w.Write(encabezado); err != nil {
		return err
	}
	for _, i := range intentos {
		exito := "0"
		if i.exito {
			exito = "1"
		}
		latencia := float64(i.respuesta.Sub(i.envio).Microseconds()) / 1000.0
		fila := []string{
			strconv.Itoa(i.secuencia),
			i.envio.UTC().Format(domain.FormatoTimestamp),
			i.respuesta.UTC().Format(domain.FormatoTimestamp),
			exito,
			i.replicaID,
			strconv.FormatFloat(latencia, 'f', 3, 64),
			strconv.Itoa(i.codigoHTTP),
			strconv.FormatFloat(i.segundoRelativo, 'f', 3, 64),
			etiqueta,
			i.detalleError,
		}
		if err := w.Write(fila); err != nil {
			return err
		}
	}
	return w.Error()
}

// imprimirResumen deja en stdout el % de exito y el reparto entre replicas.
func imprimirResumen(cfg config.Cliente, intentos []intento) {
	total := len(intentos)
	if total == 0 {
		log.Println("no se registro ningun intento")
		return
	}
	exitos := 0
	porReplica := map[string]int{}
	for _, i := range intentos {
		if i.exito {
			exitos++
			porReplica[i.replicaID]++
		}
	}
	ids := make([]string, 0, len(porReplica))
	for id := range porReplica {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	log.Printf("=== RESUMEN %s ===", cfg.Etiqueta)
	log.Printf("solicitudes: %d | exitosas: %d | fallidas: %d | %% exito: %.2f%%",
		total, exitos, total-exitos, 100*float64(exitos)/float64(total))
	for _, id := range ids {
		log.Printf("  replica %s gano %d carreras (%.1f%%)", id, porReplica[id],
			100*float64(porReplica[id])/float64(exitos))
	}
	log.Printf("CSV: %s", cfg.ArchivoCSV)
	fmt.Printf("RESUMEN %s total=%d exitos=%d porcentaje=%.2f\n", cfg.Etiqueta, total, exitos,
		100*float64(exitos)/float64(total))
}

// esperarDispatcher reintenta GET /salud hasta que el dispatcher responda.
func esperarDispatcher(ctx context.Context, urlBase string, limite time.Duration) error {
	cliente := &http.Client{Timeout: 500 * time.Millisecond}
	fin := time.Now().Add(limite)
	var ultimo error
	for time.Now().Before(fin) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		respuesta, err := cliente.Get(urlBase + "/salud")
		if err == nil {
			_, _ = io.Copy(io.Discard, respuesta.Body)
			_ = respuesta.Body.Close()
			if respuesta.StatusCode == http.StatusOK {
				return nil
			}
			ultimo = fmt.Errorf("codigo %d", respuesta.StatusCode)
		} else {
			ultimo = err
		}
		time.Sleep(250 * time.Millisecond)
	}
	return ultimo
}
