package httpin

import (
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// OpcionesRegistro configura la traza de peticiones HTTP del servidor.
//
// Se puede apagar (LOG_PETICIONES=false) para las corridas de carga: escribir
// una linea por solicitud mientras se mide la latencia agrega trabajo de E/S
// que contamina justo la medicion que se quiere tomar.
type OpcionesRegistro struct {
	Activo bool
	// IncluirVigilancia agrega a la traza el trafico de inspeccion (/config,
	// /salud), que no es trafico de negocio.
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

// ConRegistroPeticiones deja una linea por peticion atendida:
//
//	peticion GET /resumen/1004 -> 200 cache=HIT 0.2 ms 76 B cliente=172.18.0.1
//
// La etiqueta cache=HIT/MISS permite ver la tactica R2 en vivo en
// "docker compose logs": al principio casi todo es MISS y tarda ~200 ms;
// en cuanto las 10 tarjetas quedan calculadas, pasa a HIT y a decimas de ms.
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
		if origen := g.Header().Get("X-Desde-Cache"); origen != "" {
			if origen == "true" {
				detalle = " cache=HIT"
			} else {
				detalle = " cache=MISS"
			}
		}

		// Una peticion que termina sin escribir nada y con el contexto cancelado
		// es un cliente que se rindio antes de recibir respuesta: en el CSV es
		// un evento no procesado. Registrarla como 200 seria enganoso.
		resultado := strconv.Itoa(g.codigo)
		if !g.escribio && r.Context().Err() != nil {
			resultado = "abandonada"
			detalle = " (el cliente se rindio antes de la respuesta)"
		}

		registrador.Printf("peticion %s %s -> %s%s %.1f ms %d B cliente=%s",
			r.Method, r.URL.Path, resultado, detalle,
			float64(duracion.Microseconds())/1000.0, g.bytes, direccionCliente(r))
	})
}

// esRutaDeVigilancia identifica el trafico de inspeccion, que no es negocio.
func esRutaDeVigilancia(ruta string) bool {
	switch ruta {
	case "/config", "/salud":
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
