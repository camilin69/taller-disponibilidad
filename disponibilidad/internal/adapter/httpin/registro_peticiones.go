package httpin

import (
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// OpcionesRegistro configura la traza de peticiones HTTP de un servicio.
//
// Se puede apagar (LOG_PETICIONES=false) para las corridas de carga, donde
// 20 req/s durante 40 s generarian cientos de lineas que estorban al leer la
// bitacora del monitor.
type OpcionesRegistro struct {
	Activo bool
	// IncluirVigilancia agrega a la traza el trafico periodico: los sondeos del
	// monitor (GET /ping), los chequeos de salud y el refresco del panel React
	// (/estado, /estado/detalle, /bitacora, /metricas). Apagado por omision:
	// son varias lineas por segundo que ahogarian a las consultas de negocio.
	IncluirVigilancia bool
	// Registrador permite inyectar un destino distinto en las pruebas.
	Registrador *log.Logger
}

func (o OpcionesRegistro) registrador() *log.Logger {
	if o.Registrador != nil {
		return o.Registrador
	}
	return log.Default()
}

// grabadora envuelve el ResponseWriter para capturar el codigo de estado y el
// tamano de la respuesta sin alterar el comportamiento del manejador.
type grabadora struct {
	http.ResponseWriter
	codigo   int
	escribio bool
	bytes    int
}

func (g *grabadora) WriteHeader(codigo int) {
	if !g.escribio {
		g.codigo = codigo
		g.escribio = true
	}
	g.ResponseWriter.WriteHeader(codigo)
}

func (g *grabadora) Write(datos []byte) (int, error) {
	if !g.escribio {
		g.codigo = http.StatusOK
		g.escribio = true
	}
	n, err := g.ResponseWriter.Write(datos)
	g.bytes += n
	return n, err
}

// Flush conserva el streaming que usa POST /chaos/crash para responder antes
// de terminar el proceso.
func (g *grabadora) Flush() {
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// ConRegistroPeticiones deja una linea por peticion atendida:
//
//	peticion GET /saldo/1234 -> 200 replica=C 4.1 ms 118 B cliente=172.18.0.1
//
// En las replicas, las consultas que pierden la carrera aparecen como
// "cancelada (descartada por la redundancia)": el dispatcher aborto la peticion
// porque otra replica ya habia respondido.
//
// En el dispatcher la etiqueta "replica" indica cual replica gano la carrera de
// la redundancia activa, de modo que el log muestra el reparto en vivo.
func ConRegistroPeticiones(siguiente http.Handler, opts OpcionesRegistro) http.Handler {
	if !opts.Activo {
		return siguiente
	}
	registrador := opts.registrador()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !opts.IncluirVigilancia && esRutaDeVigilancia(r.URL.Path) {
			siguiente.ServeHTTP(w, r)
			return
		}

		inicio := time.Now()
		g := &grabadora{ResponseWriter: w, codigo: http.StatusOK}
		siguiente.ServeHTTP(g, r)
		duracion := time.Since(inicio)

		detalle := ""
		if id := g.Header().Get("X-Replica-Id"); id != "" {
			detalle = " replica=" + id
		}

		// Una peticion que termina sin escribir nada y con el contexto cancelado
		// es una perdedora de la carrera de redundancia: el dispatcher ya tenia
		// ganador y aborto el resto. Registrarla como 200 seria enganoso.
		resultado := strconv.Itoa(g.codigo)
		if !g.escribio && r.Context().Err() != nil {
			resultado = "cancelada"
			detalle = " (descartada por la redundancia)"
		}

		registrador.Printf("peticion %s %s -> %s%s %.1f ms %d B cliente=%s",
			r.Method, r.URL.Path, resultado, detalle,
			float64(duracion.Microseconds())/1000.0, g.bytes, direccionCliente(r))
	})
}

// esRutaDeVigilancia identifica el trafico periodico (sondeo del monitor,
// chequeos de salud y refresco del panel), que no es trafico de negocio.
func esRutaDeVigilancia(ruta string) bool {
	switch ruta {
	case "/ping", "/salud", "/estado", "/estado/detalle", "/bitacora", "/metricas":
		return true
	default:
		return false
	}
}

// direccionCliente devuelve la IP de origen de la peticion.
func direccionCliente(r *http.Request) string {
	if reenviada := r.Header.Get("X-Forwarded-For"); reenviada != "" {
		return strings.TrimSpace(strings.Split(reenviada, ",")[0])
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
