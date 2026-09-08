// Package config traduce variables de entorno a configuracion tipada.
// Ningun parametro del taller (T, t, k, m, numero de replicas, puertos) esta
// escrito como constante en la logica: todo entra por aqui.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
)

// Dispatcher agrupa la configuracion del proceso dispatcher.
type Dispatcher struct {
	Puerto          int
	Replicas        []domain.Replica
	IntervaloSondeo time.Duration // T
	TimeoutSondeo   time.Duration // t
	K               int           // fallos consecutivos para CAIDA
	M               int           // exitos consecutivos para VIVA
	EstadoInicial   domain.Estado
	TimeoutConsulta time.Duration // tiempo maximo de la carrera de redundancia
	ArchivoBitacora string
	TrazaSondeos    bool // imprime cada ping en stdout (modo depuracion)
}

// Replica agrupa la configuracion de un proceso replica.
type Replica struct {
	ID          string
	Puerto      int
	LatenciaMin time.Duration // latencia artificial minima de /saldo
	LatenciaMax time.Duration // latencia artificial maxima de /saldo
	SaldoBase   int64
}

// Cliente agrupa la configuracion del generador de carga.
type Cliente struct {
	URLDispatcher string
	TasaRPS       int
	Duracion      time.Duration
	IDTarjeta     string
	ArchivoCSV    string
	Timeout       time.Duration
	Etiqueta      string // E0 / E1: identifica el experimento en el CSV
	EsperaInicial time.Duration
}

// Inyector agrupa la configuracion del inyector de fallas.
type Inyector struct {
	Objetivo      string // nombre del contenedor o id de replica
	Modo          string // "docker" o "http"
	URLReplica    string // usada en modo http (POST /chaos/crash)
	ArchivoLog    string
	ComandoDocker string // "kill" o "stop"
}

// CargarDispatcher lee la configuracion del dispatcher desde el entorno.
func CargarDispatcher() (Dispatcher, error) {
	cfg := Dispatcher{
		Puerto:          entero("PUERTO", 8080),
		IntervaloSondeo: duracionMS("MONITOR_T_MS", 1000),
		TimeoutSondeo:   duracionMS("MONITOR_TIMEOUT_MS", 300),
		K:               entero("MONITOR_K", 2),
		M:               entero("MONITOR_M", 2),
		TimeoutConsulta: duracionMS("CONSULTA_TIMEOUT_MS", 1500),
		ArchivoBitacora: texto("BITACORA_ARCHIVO", "/datos/monitor.log"),
		TrazaSondeos:    booleano("MONITOR_TRAZA", false),
	}

	estado, err := domain.ParsearEstado(texto("ESTADO_INICIAL", string(domain.EstadoViva)))
	if err != nil {
		return Dispatcher{}, fmt.Errorf("ESTADO_INICIAL: %w", err)
	}
	cfg.EstadoInicial = estado

	replicas, err := ParsearReplicas(texto("REPLICAS", ""))
	if err != nil {
		return Dispatcher{}, err
	}
	cfg.Replicas = replicas

	return cfg, cfg.Validar()
}

// Validar comprueba que la configuracion sea coherente antes de arrancar.
func (c Dispatcher) Validar() error {
	if len(c.Replicas) < 2 {
		return fmt.Errorf("se requieren al menos 2 replicas (REPLICAS), se configuraron %d", len(c.Replicas))
	}
	if c.K < 1 || c.M < 1 {
		return fmt.Errorf("MONITOR_K y MONITOR_M deben ser >= 1 (k=%d, m=%d)", c.K, c.M)
	}
	if c.TimeoutSondeo >= c.IntervaloSondeo {
		return fmt.Errorf("MONITOR_TIMEOUT_MS (%v) debe ser menor que MONITOR_T_MS (%v)", c.TimeoutSondeo, c.IntervaloSondeo)
	}
	if c.Puerto <= 0 || c.Puerto > 65535 {
		return fmt.Errorf("PUERTO invalido: %d", c.Puerto)
	}
	return nil
}

// DeteccionEsperada es la cota teorica del tiempo de deteccion: k ciclos de T.
// Se publica en GET /estado para comparar contra la medicion experimental.
func (c Dispatcher) DeteccionEsperada() time.Duration {
	return time.Duration(c.K) * c.IntervaloSondeo
}

// ParsearReplicas interpreta la lista de replicas del entorno. Formato:
//
//	REPLICAS="A=http://replica-a:8080,B=http://replica-b:8080"
//
// Tambien se admite "A|http://..." y omitir el identificador, en cuyo caso se
// deduce del host. Agregar una replica es editar esta variable: el codigo del
// dispatcher no cambia.
func ParsearReplicas(valor string) ([]domain.Replica, error) {
	crudo := strings.TrimSpace(valor)
	if crudo == "" {
		return nil, fmt.Errorf("la variable REPLICAS es obligatoria (ej: A=http://replica-a:8080,B=http://replica-b:8080)")
	}

	var replicas []domain.Replica
	vistos := map[string]bool{}
	for _, item := range strings.Split(crudo, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		var id, url string
		if i := strings.IndexAny(item, "=|"); i > 0 {
			id = strings.TrimSpace(item[:i])
			url = strings.TrimSpace(item[i+1:])
		} else {
			url = item
			id = idDesdeURL(item)
		}
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			url = "http://" + url
		}
		r := domain.Replica{ID: strings.ToUpper(id), URLBase: strings.TrimRight(url, "/")}
		if err := r.Valida(); err != nil {
			return nil, fmt.Errorf("REPLICAS: %w", err)
		}
		if vistos[r.ID] {
			return nil, fmt.Errorf("REPLICAS: identificador duplicado %q", r.ID)
		}
		vistos[r.ID] = true
		replicas = append(replicas, r)
	}
	if len(replicas) == 0 {
		return nil, fmt.Errorf("REPLICAS no contiene ninguna entrada valida")
	}
	return replicas, nil
}

// idDesdeURL deduce "A" de "http://replica-a:8080".
func idDesdeURL(url string) string {
	limpio := strings.TrimPrefix(strings.TrimPrefix(url, "http://"), "https://")
	if i := strings.IndexAny(limpio, ":/"); i > 0 {
		limpio = limpio[:i]
	}
	if i := strings.LastIndex(limpio, "-"); i >= 0 && i+1 < len(limpio) {
		limpio = limpio[i+1:]
	}
	return strings.ToUpper(limpio)
}

// CargarReplica lee la configuracion de un proceso replica.
func CargarReplica() (Replica, error) {
	cfg := Replica{
		ID:          strings.ToUpper(texto("REPLICA_ID", "")),
		Puerto:      entero("PUERTO", 8080),
		LatenciaMin: duracionMS("LATENCIA_MIN_MS", 2),
		LatenciaMax: duracionMS("LATENCIA_MAX_MS", 25),
		SaldoBase:   int64(entero("SALDO_BASE", 15000)),
	}
	if cfg.ID == "" {
		return Replica{}, fmt.Errorf("REPLICA_ID es obligatoria")
	}
	if cfg.LatenciaMax < cfg.LatenciaMin {
		cfg.LatenciaMax = cfg.LatenciaMin
	}
	if cfg.Puerto <= 0 || cfg.Puerto > 65535 {
		return Replica{}, fmt.Errorf("PUERTO invalido: %d", cfg.Puerto)
	}
	return cfg, nil
}

// CargarCliente lee la configuracion del generador de carga.
func CargarCliente() (Cliente, error) {
	cfg := Cliente{
		URLDispatcher: strings.TrimRight(texto("DISPATCHER_URL", "http://localhost:8080"), "/"),
		TasaRPS:       entero("TASA_RPS", 20),
		Duracion:      time.Duration(entero("DURACION_S", 40)) * time.Second,
		IDTarjeta:     texto("ID_TARJETA", "1234"),
		ArchivoCSV:    texto("CSV_SALIDA", "/datos/cliente.csv"),
		Timeout:       duracionMS("CLIENTE_TIMEOUT_MS", 2000),
		Etiqueta:      texto("ETIQUETA", "E0"),
		EsperaInicial: duracionMS("ESPERA_INICIAL_MS", 0),
	}
	if cfg.TasaRPS <= 0 {
		return Cliente{}, fmt.Errorf("TASA_RPS debe ser mayor que 0")
	}
	if cfg.Duracion <= 0 {
		return Cliente{}, fmt.Errorf("DURACION_S debe ser mayor que 0")
	}
	return cfg, nil
}

// CargarInyector lee la configuracion del inyector de fallas.
func CargarInyector() Inyector {
	return Inyector{
		Objetivo:      texto("OBJETIVO", ""),
		Modo:          strings.ToLower(texto("MODO", "docker")),
		URLReplica:    texto("URL_REPLICA", ""),
		ArchivoLog:    texto("INYECTOR_LOG", "evidencia/inyector.log"),
		ComandoDocker: strings.ToLower(texto("COMANDO_DOCKER", "kill")),
	}
}

// --- utilidades de lectura del entorno ---

func texto(clave, porDefecto string) string {
	if v, ok := os.LookupEnv(clave); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return porDefecto
}

func entero(clave string, porDefecto int) int {
	if v, ok := os.LookupEnv(clave); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return porDefecto
}

func duracionMS(clave string, porDefectoMS int) time.Duration {
	return time.Duration(entero(clave, porDefectoMS)) * time.Millisecond
}

func booleano(clave string, porDefecto bool) bool {
	if v, ok := os.LookupEnv(clave); ok {
		if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
			return b
		}
	}
	return porDefecto
}
