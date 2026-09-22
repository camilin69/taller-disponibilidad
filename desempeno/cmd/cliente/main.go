// Comando cliente: script de carga que simula la rafaga de usuarios que
// consultan su resumen mensual al inicio de mes. Reutiliza un conjunto pequeno
// de tarjetas para que las consultas se repitan y el cache tenga oportunidad de
// responder. Por cada intento escribe una fila del CSV que es la evidencia de
// las cuatro medidas de desempeno del taller.
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

	"github.com/camilin69/taller-desempeno/internal/domain"
	"github.com/camilin69/taller-desempeno/internal/infra/config"
)

// intento es el registro de una solicitud al servidor.
type intento struct {
	secuencia       int
	envio           time.Time
	respuesta       time.Time
	exito           bool
	desdeCache      bool
	idTarjeta       string
	codigoHTTP      int
	detalleError    string
	segundoRelativo float64
}

// latencia es el tiempo entre que la solicitud sale y llega la respuesta.
func (i intento) latencia() time.Duration { return i.respuesta.Sub(i.envio) }

// cuerpoResumen es la parte de la respuesta del servidor que interesa al CSV.
type cuerpoResumen struct {
	IDTarjeta   string `json:"id_tarjeta"`
	TotalViajes int    `json:"total_viajes"`
	GastoTotal  int64  `json:"gasto_total"`
	DesdeCache  bool   `json:"desde_cache"`
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

	if err := esperarServidor(ctx, cfg.URLServidor, 60*time.Second); err != nil {
		log.Fatalf("el servidor no quedo disponible: %v", err)
	}
	if cfg.EsperaInicial > 0 {
		log.Printf("espera inicial de %v antes de generar carga", cfg.EsperaInicial)
		time.Sleep(cfg.EsperaInicial)
	}

	tarjetas := cfg.Tarjetas()
	log.Printf("experimento=%s tasa=%d req/s duracion=%v -> %d solicitudes",
		cfg.Etiqueta, cfg.TasaRPS, cfg.Duracion, cfg.SolicitudesEsperadas())
	log.Printf("tarjetas=%d (%s ... %s) viajes=%d timeout=%v destino=%s",
		len(tarjetas), tarjetas[0], tarjetas[len(tarjetas)-1], cfg.Viajes, cfg.Timeout, cfg.URLServidor)

	intentos := generarRafaga(ctx, cfg, tarjetas)

	sort.Slice(intentos, func(i, j int) bool { return intentos[i].secuencia < intentos[j].secuencia })
	if err := escribirCSV(cfg.ArchivoCSV, cfg.Etiqueta, intentos); err != nil {
		log.Fatalf("no se pudo escribir el CSV: %v", err)
	}
	imprimirResumen(cfg, intentos)
}

// generarRafaga dispara solicitudes a ritmo alto durante la duracion fijada,
// rotando el conjunto pequeno de tarjetas.
//
// Cada solicitud corre en su propia goroutine: el cliente NO espera la
// respuesta anterior para enviar la siguiente. Eso es lo que hace que la
// rafaga llegue de verdad concurrente al servidor y que en E0 (W=1) se vea la
// cola crecer, en vez de auto-limitarse al ritmo del servidor.
func generarRafaga(ctx context.Context, cfg config.Cliente, tarjetas []string) []intento {
	esperadas := cfg.SolicitudesEsperadas()
	cliente := &http.Client{
		Timeout: cfg.Timeout,
		Transport: &http.Transport{
			// La rafaga necesita muchas conexiones simultaneas: en E0 casi
			// ninguna se libera rapido porque el servidor las tiene en cola.
			MaxIdleConns:        esperadas,
			MaxIdleConnsPerHost: esperadas,
			MaxConnsPerHost:     esperadas,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	intervalo := time.Second / time.Duration(cfg.TasaRPS)
	ticker := time.NewTicker(intervalo)
	defer ticker.Stop()

	inicio := time.Now()
	fin := inicio.Add(cfg.Duracion)

	var (
		mu         sync.Mutex
		wg         sync.WaitGroup
		resultados []intento
		secuencia  int
	)

	for time.Now().Before(fin) {
		select {
		case <-ctx.Done():
			log.Println("rafaga interrumpida por senal")
			goto esperar
		case <-ticker.C:
			secuencia++
			n := secuencia
			// Rotacion circular: con 10 tarjetas, la solicitud 11 repite la
			// tarjeta de la solicitud 1. Esa repeticion es la que el cache
			// aprovecha en E2.
			tarjeta := tarjetas[(n-1)%len(tarjetas)]
			wg.Add(1)
			go func() {
				defer wg.Done()
				res := ejecutarIntento(cliente, cfg, tarjeta, n, inicio)
				mu.Lock()
				resultados = append(resultados, res)
				mu.Unlock()
			}()
			if secuencia%(cfg.TasaRPS*5) == 0 {
				log.Printf("progreso: %d solicitudes enviadas (%.0f s)", secuencia, time.Since(inicio).Seconds())
			}
		}
	}

esperar:
	wg.Wait()
	return resultados
}

// ejecutarIntento realiza una consulta y clasifica su resultado.
//
// Una solicitud que vence el tiempo limite queda con exito=0: es un EVENTO NO
// PROCESADO, la cuarta medida de desempeno.
func ejecutarIntento(cliente *http.Client, cfg config.Cliente, tarjeta string, secuencia int, inicioRafaga time.Time) intento {
	envio := time.Now()
	res := intento{
		secuencia:       secuencia,
		envio:           envio,
		idTarjeta:       tarjeta,
		segundoRelativo: envio.Sub(inicioRafaga).Seconds(),
	}

	url := fmt.Sprintf("%s/resumen/%s?viajes=%d", cfg.URLServidor, tarjeta, cfg.Viajes)
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
	var cuerpo cuerpoResumen
	errCuerpo := json.NewDecoder(io.LimitReader(respuesta.Body, 8192)).Decode(&cuerpo)
	res.respuesta = time.Now()

	if respuesta.StatusCode != http.StatusOK {
		res.detalleError = "codigo " + strconv.Itoa(respuesta.StatusCode)
		if respuesta.StatusCode == http.StatusServiceUnavailable {
			res.detalleError = "servidor saturado (cola llena)"
		}
		return res
	}
	if errCuerpo != nil {
		res.detalleError = "cuerpo ilegible: " + errCuerpo.Error()
		return res
	}

	res.desdeCache = cuerpo.DesdeCache
	res.exito = cuerpo.TotalViajes > 0 && cuerpo.GastoTotal > 0
	if !res.exito {
		res.detalleError = "resumen incompleto en la respuesta"
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
		"desde_cache", "id_tarjeta", "latencia_ms", "codigo_http", "segundo_relativo",
		"experimento", "error"}
	if err := w.Write(encabezado); err != nil {
		return err
	}
	for _, i := range intentos {
		fila := []string{
			strconv.Itoa(i.secuencia),
			i.envio.UTC().Format(domain.FormatoTimestamp),
			i.respuesta.UTC().Format(domain.FormatoTimestamp),
			bit(i.exito),
			bit(i.desdeCache),
			i.idTarjeta,
			strconv.FormatFloat(float64(i.latencia().Microseconds())/1000.0, 'f', 3, 64),
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

// bit convierte un booleano en la columna 0/1 que espera el CSV.
func bit(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

// imprimirResumen deja en stdout un adelanto de las metricas; la tabla formal
// la calcula el comando metricas a partir del CSV.
func imprimirResumen(cfg config.Cliente, intentos []intento) {
	total := len(intentos)
	if total == 0 {
		log.Println("no se registro ningun intento")
		return
	}

	var (
		exitosas   int
		desdeCache int
		latencias  []float64
	)
	for _, i := range intentos {
		if !i.exito {
			continue
		}
		exitosas++
		if i.desdeCache {
			desdeCache++
		}
		latencias = append(latencias, float64(i.latencia().Microseconds())/1000.0)
	}

	promedio := 0.0
	for _, l := range latencias {
		promedio += l
	}
	if len(latencias) > 0 {
		promedio /= float64(len(latencias))
	}

	log.Printf("=== RESUMEN %s ===", cfg.Etiqueta)
	log.Printf("solicitudes: %d | exitosas: %d | no procesadas: %d (%.2f%%)",
		total, exitosas, total-exitosas, 100*float64(total-exitosas)/float64(total))
	log.Printf("latencia promedio: %.1f ms | throughput: %.2f req/s",
		promedio, float64(exitosas)/cfg.Duracion.Seconds())
	log.Printf("desde cache: %d de %d exitosas (%.2f%%)", desdeCache, exitosas,
		porcentaje(desdeCache, exitosas))
	log.Printf("CSV: %s", cfg.ArchivoCSV)
	fmt.Printf("RESUMEN %s total=%d exitosas=%d cache=%.2f%% latencia_media=%.1fms\n",
		cfg.Etiqueta, total, exitosas, porcentaje(desdeCache, exitosas), promedio)
}

func porcentaje(parte, total int) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(parte) / float64(total)
}

// esperarServidor reintenta GET /salud hasta que el servidor responda.
func esperarServidor(ctx context.Context, urlBase string, limite time.Duration) error {
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
