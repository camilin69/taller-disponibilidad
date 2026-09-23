// Pruebas de INTEGRACION del sistema RECAUDO-T.
//
// Levantan replicas reales (servidores HTTP con el mismo manejador que corre en
// cada contenedor) y el dispatcher real completo (monitor Ping/Echo, registro,
// redundancia activa, bitacora en archivo). Reproducen en pequeno el protocolo
// experimental del taller: E0 (linea base) y E1 (caida abrupta).
package integracion

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/adapter/httpin"
	"github.com/camilin69/taller-disponibilidad/internal/domain"
	"github.com/camilin69/taller-disponibilidad/internal/infra/config"
	"github.com/camilin69/taller-disponibilidad/internal/infra/ensamblaje"
)

// entorno agrupa el sistema completo levantado para una prueba.
type entorno struct {
	replicas   map[string]*httptest.Server
	dispatcher *httptest.Server
	sistema    *ensamblaje.Sistema
	bitacora   string
	cancelar   context.CancelFunc
}

// levantarSistema crea las replicas indicadas y el dispatcher que las vigila.
func levantarSistema(t *testing.T, ids []string, cfgBase config.Dispatcher) *entorno {
	t.Helper()

	env := &entorno{replicas: map[string]*httptest.Server{}}
	var replicas []domain.Replica
	for _, id := range ids {
		api := httpin.NuevaReplica(id, 15000, time.Millisecond, 20*time.Millisecond)
		srv := httptest.NewServer(api.Rutas())
		env.replicas[id] = srv
		replicas = append(replicas, domain.Replica{ID: id, URLBase: srv.URL})
	}

	cfg := cfgBase
	cfg.Replicas = replicas
	cfg.ArchivoBitacora = filepath.Join(t.TempDir(), "monitor.log")
	env.bitacora = cfg.ArchivoBitacora

	sistema, err := ensamblaje.ArmarDispatcher(cfg)
	if err != nil {
		t.Fatalf("no se pudo armar el dispatcher: %v", err)
	}
	env.sistema = sistema

	ctx, cancelar := context.WithCancel(context.Background())
	env.cancelar = cancelar
	sistema.IniciarMonitor(ctx)
	env.dispatcher = httptest.NewServer(sistema.API)

	t.Cleanup(func() {
		env.dispatcher.Close()
		cancelar()
		sistema.Monitor.Esperar()
		_ = sistema.Cerrar()
		for _, srv := range env.replicas {
			srv.Close()
		}
	})
	return env
}

// configProduccion usa exactamente los parametros del docker-compose:
// T=1s, t=300ms, k=2, m=2 (deteccion esperada ~2 s, limite del taller 3 s).
func configProduccion() config.Dispatcher {
	return config.Dispatcher{
		// El puerto real lo asigna httptest; se fija uno valido para pasar la
		// validacion de configuracion.
		Puerto:          8080,
		IntervaloSondeo: time.Second,
		TimeoutSondeo:   300 * time.Millisecond,
		K:               2,
		M:               2,
		EstadoInicial:   domain.EstadoViva,
		TimeoutConsulta: 1500 * time.Millisecond,
	}
}

// resultadoCarga resume una tanda de solicitudes del cliente simulado.
type resultadoCarga struct {
	total       int
	exitosas    int
	fallidas    int
	porReplica  map[string]int
	primerFallo time.Time
}

// generarCarga simula al cliente: tasa constante durante la duracion indicada.
func generarCarga(t *testing.T, urlBase string, rps int, duracion time.Duration) resultadoCarga {
	t.Helper()

	cliente := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(time.Second / time.Duration(rps))
	defer ticker.Stop()
	fin := time.Now().Add(duracion)

	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		res = resultadoCarga{porReplica: map[string]int{}}
	)

	for time.Now().Before(fin) {
		<-ticker.C
		wg.Add(1)
		go func() {
			defer wg.Done()
			envio := time.Now()
			respuesta, err := cliente.Get(urlBase + "/saldo/1234")
			mu.Lock()
			defer mu.Unlock()
			res.total++
			if err != nil || respuesta.StatusCode != http.StatusOK {
				res.fallidas++
				if res.primerFallo.IsZero() {
					res.primerFallo = envio
				}
				if respuesta != nil {
					_, _ = io.Copy(io.Discard, respuesta.Body)
					_ = respuesta.Body.Close()
				}
				return
			}
			var cuerpo httpin.RespuestaSaldo
			_ = json.NewDecoder(respuesta.Body).Decode(&cuerpo)
			_ = respuesta.Body.Close()
			if cuerpo.ReplicaID == "" || cuerpo.Saldo <= 0 {
				res.fallidas++
				return
			}
			res.exitosas++
			res.porReplica[cuerpo.ReplicaID]++
		}()
	}
	wg.Wait()
	return res
}

// E0 - LINEA BASE: sin fallas, 100% de exito y mas de una replica respondiendo
// (evidencia de que la redundancia realmente compite).
func TestE0LineaBaseSinFallas(t *testing.T) {
	env := levantarSistema(t, []string{"A", "B", "C"}, configProduccion())

	res := generarCarga(t, env.dispatcher.URL, 20, 3*time.Second)

	if res.total == 0 {
		t.Fatal("no se genero carga")
	}
	if res.fallidas != 0 {
		t.Fatalf("E0 deberia tener 0 fallos, hubo %d de %d", res.fallidas, res.total)
	}
	if len(res.porReplica) < 2 {
		t.Fatalf("solo respondio %d replica(s): la redundancia no esta compitiendo (%v)",
			len(res.porReplica), res.porReplica)
	}
	t.Logf("E0: %d solicitudes, 100%% exito, reparto %v", res.total, res.porReplica)
}

// E1 - CAIDA ABRUPTA: se mata una replica en medio de la carga.
//
//	(a) el monitor detecta la caida en <= 3 s y la registra en la bitacora;
//	(b) el cliente NO percibe errores gracias a la redundancia activa.
func TestE1CaidaAbruptaDeteccionYTransparencia(t *testing.T) {
	env := levantarSistema(t, []string{"A", "B", "C"}, configProduccion())

	// Se deja estabilizar el monitor antes de inyectar la falla.
	time.Sleep(1200 * time.Millisecond)

	var res resultadoCarga
	listo := make(chan struct{})
	go func() {
		defer close(listo)
		res = generarCarga(t, env.dispatcher.URL, 20, 6*time.Second)
	}()

	time.Sleep(1500 * time.Millisecond)
	// Inyeccion de la falla: se registra el timestamp ANTES de matar la replica.
	momentoInyeccion := time.Now()
	env.replicas["B"].CloseClientConnections()
	env.replicas["B"].Close()
	t.Logf("replica B terminada a las %s", momentoInyeccion.Format(domain.FormatoTimestamp))

	// (a) Deteccion por Ping/Echo.
	if !esperar(4*time.Second, func() bool {
		estado, _ := env.sistema.Registro.EstadoDe("B")
		return estado == domain.EstadoCaida
	}) {
		t.Fatal("el monitor no detecto la caida de B")
	}
	deteccion := time.Since(momentoInyeccion)
	if deteccion > 3*time.Second {
		t.Fatalf("tiempo de deteccion %v > 3 s exigidos por el escenario de calidad", deteccion)
	}
	t.Logf("tiempo de deteccion medido: %.3f s", deteccion.Seconds())

	// GET /estado debe reflejar el estado real.
	estados := consultarEstado(t, env.dispatcher.URL)
	if estados["B"] != string(domain.EstadoCaida) {
		t.Fatalf("GET /estado no refleja la caida: %v", estados)
	}
	if estados["A"] != string(domain.EstadoViva) || estados["C"] != string(domain.EstadoViva) {
		t.Fatalf("las replicas sanas no deben marcarse CAIDA: %v", estados)
	}

	// La bitacora persistida debe contener la transicion.
	contenido, err := os.ReadFile(env.bitacora)
	if err != nil {
		t.Fatalf("no se pudo leer la bitacora: %v", err)
	}
	if !strings.Contains(string(contenido), "REPLICA_B VIVA -> CAIDA") {
		t.Fatalf("la bitacora no registro la transicion:\n%s", contenido)
	}

	<-listo

	// (b) Transparencia para el cliente: la redundancia activa lo protegio.
	if res.fallidas > 0 {
		t.Fatalf("el cliente percibio %d errores de %d durante la caida", res.fallidas, res.total)
	}
	if res.porReplica["B"] == 0 {
		t.Fatal("B nunca respondio antes de caer: la prueba no ejercito la redundancia")
	}
	t.Logf("E1: %d solicitudes, %d exitosas (100%%), reparto %v", res.total, res.exitosas, res.porReplica)
}

// Tras recuperar la replica, m ecos consecutivos la devuelven a VIVA.
func TestReplicaSeRecuperaYVuelveAViva(t *testing.T) {
	cfg := configProduccion()
	cfg.IntervaloSondeo = 200 * time.Millisecond
	cfg.TimeoutSondeo = 100 * time.Millisecond
	env := levantarSistema(t, []string{"A", "B"}, cfg)

	// Se tumba B reemplazando su servidor por uno cerrado.
	env.replicas["B"].Close()
	if !esperar(3*time.Second, func() bool {
		e, _ := env.sistema.Registro.EstadoDe("B")
		return e == domain.EstadoCaida
	}) {
		t.Fatal("B no fue marcada CAIDA")
	}

	// Se levanta de nuevo B en la MISMA direccion (como haria docker start).
	direccion := strings.TrimPrefix(env.replicas["B"].URL, "http://")
	api := httpin.NuevaReplica("B", 15000, time.Millisecond, 5*time.Millisecond)
	nuevo := httptest.NewUnstartedServer(api.Rutas())
	oyente, err := escucharEn(direccion)
	if err != nil {
		t.Skipf("no se pudo reutilizar el puerto %s: %v", direccion, err)
	}
	nuevo.Listener = oyente
	nuevo.Start()
	defer nuevo.Close()

	if !esperar(3*time.Second, func() bool {
		e, _ := env.sistema.Registro.EstadoDe("B")
		return e == domain.EstadoViva
	}) {
		t.Fatal("B no volvio a VIVA tras recuperarse")
	}
	contenido, _ := os.ReadFile(env.bitacora)
	if !strings.Contains(string(contenido), "REPLICA_B CAIDA -> VIVA") {
		t.Fatalf("la recuperacion no quedo en bitacora:\n%s", contenido)
	}
}

// Si TODAS las replicas caen, el dispatcher responde 503 y no se cuelga.
func TestSinReplicasVivasElDispatcherResponde503(t *testing.T) {
	cfg := configProduccion()
	cfg.IntervaloSondeo = 200 * time.Millisecond
	cfg.TimeoutSondeo = 100 * time.Millisecond
	env := levantarSistema(t, []string{"A", "B"}, cfg)

	for _, srv := range env.replicas {
		srv.Close()
	}
	if !esperar(3*time.Second, func() bool {
		estados := consultarEstadoSilencioso(env.dispatcher.URL)
		return estados["A"] == string(domain.EstadoCaida) && estados["B"] == string(domain.EstadoCaida)
	}) {
		t.Fatal("el monitor no marco todas las replicas como CAIDA")
	}

	respuesta, err := http.Get(env.dispatcher.URL + "/saldo/1234")
	if err != nil {
		t.Fatalf("el dispatcher no respondio: %v", err)
	}
	defer func() { _ = respuesta.Body.Close() }()
	if respuesta.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("codigo %d, se esperaba 503", respuesta.StatusCode)
	}
}

// El contrato de GET /estado es {"A":"VIVA","B":"CAIDA"}.
func TestContratoDeEstado(t *testing.T) {
	env := levantarSistema(t, []string{"A", "B"}, configProduccion())
	estados := consultarEstado(t, env.dispatcher.URL)
	if len(estados) != 2 || estados["A"] != "VIVA" || estados["B"] != "VIVA" {
		t.Fatalf("contrato de /estado inesperado: %v", estados)
	}
}

// Agregar una cuarta replica solo requiere configuracion (R2).
func TestAgregarUnaCuartaReplicaSinTocarCodigo(t *testing.T) {
	env := levantarSistema(t, []string{"A", "B", "C", "D"}, configProduccion())
	estados := consultarEstado(t, env.dispatcher.URL)
	if len(estados) != 4 || estados["D"] != "VIVA" {
		t.Fatalf("la cuarta replica no quedo registrada: %v", estados)
	}
	res := generarCarga(t, env.dispatcher.URL, 20, time.Second)
	if res.fallidas != 0 {
		t.Fatalf("con 4 replicas hubo %d fallos", res.fallidas)
	}
}

// El gancho de caos de la replica responde 200 y pide terminar el proceso.
func TestGanchoDeCaosDeLaReplica(t *testing.T) {
	api := httpin.NuevaReplica("Z", 15000, 0, 0)
	terminado := make(chan int, 1)
	api.Salir = func(codigo int) { terminado <- codigo }
	srv := httptest.NewServer(api.Rutas())
	defer srv.Close()

	respuesta, err := http.Post(srv.URL+"/chaos/crash", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /chaos/crash: %v", err)
	}
	defer func() { _ = respuesta.Body.Close() }()
	if respuesta.StatusCode != http.StatusOK {
		t.Fatalf("codigo %d, se esperaba 200", respuesta.StatusCode)
	}
	select {
	case codigo := <-terminado:
		if codigo != 137 {
			t.Fatalf("codigo de salida %d, se esperaba 137", codigo)
		}
	case <-time.After(time.Second):
		t.Fatal("la replica no invoco la terminacion del proceso")
	}
}

// --- utilidades ---

func consultarEstado(t *testing.T, urlBase string) map[string]string {
	t.Helper()
	respuesta, err := http.Get(urlBase + "/estado")
	if err != nil {
		t.Fatalf("GET /estado: %v", err)
	}
	defer func() { _ = respuesta.Body.Close() }()
	if respuesta.StatusCode != http.StatusOK {
		t.Fatalf("GET /estado devolvio %d", respuesta.StatusCode)
	}
	var estados map[string]string
	if err := json.NewDecoder(respuesta.Body).Decode(&estados); err != nil {
		t.Fatalf("cuerpo de /estado ilegible: %v", err)
	}
	return estados
}

func consultarEstadoSilencioso(urlBase string) map[string]string {
	respuesta, err := http.Get(urlBase + "/estado")
	if err != nil {
		return nil
	}
	defer func() { _ = respuesta.Body.Close() }()
	var estados map[string]string
	_ = json.NewDecoder(respuesta.Body).Decode(&estados)
	return estados
}

func esperar(limite time.Duration, cond func() bool) bool {
	fin := time.Now().Add(limite)
	for time.Now().Before(fin) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

func escucharEn(direccion string) (net.Listener, error) {
	if direccion == "" {
		return nil, fmt.Errorf("direccion vacia")
	}
	return net.Listen("tcp", direccion)
}
