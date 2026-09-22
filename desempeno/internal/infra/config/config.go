// Package config traduce variables de entorno a configuracion tipada.
// Ningun parametro del taller (W, capacidad de la cola, costo del calculo,
// cache activo, tasa y duracion de la carga) esta escrito como constante en la
// logica: todo entra por aqui, que es lo que permite correr E0, E1 y E2 con el
// mismo binario.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Servidor agrupa la configuracion del proceso servidor.
type Servidor struct {
	Puerto           int
	Trabajadores     int           // W: tamano del pool (R1)
	CapacidadCola    int           // solicitudes que pueden esperar turno
	CacheActiva      bool          // enciende o apaga la tactica R2
	CostoCalculo     time.Duration // duracion fija del calculo costoso
	ViajesPorDefecto int           // N usado cuando la query no trae ?viajes=
	LogPeticiones    bool
	LogVigilancia    bool
}

// Cliente agrupa la configuracion del generador de carga.
type Cliente struct {
	URLServidor   string
	TasaRPS       int
	Duracion      time.Duration
	NumTarjetas   int // cuantas tarjetas distintas rota la rafaga
	TarjetaBase   int // primer identificador de tarjeta
	Viajes        int // N que se pide en ?viajes=
	ArchivoCSV    string
	Timeout       time.Duration // tiempo limite: mas alla es evento no procesado
	Etiqueta      string        // E0 / E1 / E2: identifica el experimento en el CSV
	EsperaInicial time.Duration
}

// CargarServidor lee la configuracion del servidor desde el entorno.
func CargarServidor() (Servidor, error) {
	cfg := Servidor{
		Puerto:           entero("PUERTO", 8080),
		Trabajadores:     entero("TRABAJADORES", 1),
		CapacidadCola:    entero("COLA_MAX", 256),
		CacheActiva:      booleano("CACHE_ACTIVA", false),
		CostoCalculo:     duracionMS("COSTO_CALCULO_MS", 200),
		ViajesPorDefecto: entero("VIAJES_POR_DEFECTO", 40),
		LogPeticiones:    booleano("LOG_PETICIONES", true),
		LogVigilancia:    booleano("LOG_VIGILANCIA", false),
	}
	return cfg, cfg.Validar()
}

// Validar comprueba que la configuracion sea coherente antes de arrancar.
func (c Servidor) Validar() error {
	if c.Trabajadores < 1 {
		return fmt.Errorf("TRABAJADORES debe ser >= 1, se configuro %d", c.Trabajadores)
	}
	if c.CapacidadCola < 1 {
		return fmt.Errorf("COLA_MAX debe ser >= 1, se configuro %d", c.CapacidadCola)
	}
	if c.CostoCalculo <= 0 {
		return fmt.Errorf("COSTO_CALCULO_MS debe ser mayor que 0, se configuro %v", c.CostoCalculo)
	}
	if c.ViajesPorDefecto < 1 {
		return fmt.Errorf("VIAJES_POR_DEFECTO debe ser >= 1, se configuro %d", c.ViajesPorDefecto)
	}
	if c.Puerto <= 0 || c.Puerto > 65535 {
		return fmt.Errorf("PUERTO invalido: %d", c.Puerto)
	}
	return nil
}

// CapacidadTeorica es el techo de throughput del servidor: W trabajadores que
// tardan CostoCalculo cada uno. Con W=1 y 200 ms son 5 solicitudes por segundo;
// con W=8, 40. Se publica junto al resumen del arranque para poder anticipar si
// la carga configurada va a saturar al servidor (que es justo lo que E0 busca).
func (c Servidor) CapacidadTeorica() float64 {
	return float64(c.Trabajadores) / c.CostoCalculo.Seconds()
}

// CargarCliente lee la configuracion del generador de carga.
func CargarCliente() (Cliente, error) {
	cfg := Cliente{
		URLServidor:   strings.TrimRight(texto("SERVIDOR_URL", "http://localhost:8080"), "/"),
		TasaRPS:       entero("TASA_RPS", 8),
		Duracion:      time.Duration(entero("DURACION_S", 16)) * time.Second,
		NumTarjetas:   entero("NUM_TARJETAS", 10),
		TarjetaBase:   entero("TARJETA_BASE", 1000),
		Viajes:        entero("VIAJES", 40),
		ArchivoCSV:    texto("CSV_SALIDA", "/datos/cliente.csv"),
		Timeout:       duracionMS("CLIENTE_TIMEOUT_MS", 3000),
		Etiqueta:      texto("ETIQUETA", "E0"),
		EsperaInicial: duracionMS("ESPERA_INICIAL_MS", 0),
	}
	if cfg.TasaRPS <= 0 {
		return Cliente{}, fmt.Errorf("TASA_RPS debe ser mayor que 0")
	}
	if cfg.Duracion <= 0 {
		return Cliente{}, fmt.Errorf("DURACION_S debe ser mayor que 0")
	}
	if cfg.NumTarjetas <= 0 {
		return Cliente{}, fmt.Errorf("NUM_TARJETAS debe ser mayor que 0")
	}
	if cfg.Viajes <= 0 {
		return Cliente{}, fmt.Errorf("VIAJES debe ser mayor que 0")
	}
	return cfg, nil
}

// Tarjetas construye el conjunto pequeno de identificadores que la rafaga
// reutiliza. Que sean pocos es deliberado: es la condicion que le da al cache
// la oportunidad de responder (experimento E2).
func (c Cliente) Tarjetas() []string {
	ids := make([]string, 0, c.NumTarjetas)
	for i := 0; i < c.NumTarjetas; i++ {
		ids = append(ids, strconv.Itoa(c.TarjetaBase+i))
	}
	return ids
}

// SolicitudesEsperadas es cuantas solicitudes dispara la corrida configurada.
func (c Cliente) SolicitudesEsperadas() int {
	return int(c.Duracion.Seconds()) * c.TasaRPS
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
