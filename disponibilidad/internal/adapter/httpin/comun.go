// Package httpin contiene los adaptadores de ENTRADA por HTTP: convierten
// peticiones en llamadas a casos de uso y respuestas de dominio en JSON.
package httpin

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// ErrorHTTP es el cuerpo uniforme de error de toda la API.
type ErrorHTTP struct {
	Error   string `json:"error"`
	Detalle string `json:"detalle,omitempty"`
}

// escribirJSON serializa el cuerpo con el codigo indicado.
func escribirJSON(w http.ResponseWriter, codigo int, cuerpo any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(codigo)
	if cuerpo == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(cuerpo)
}

// escribirError responde un error con formato uniforme.
func escribirError(w http.ResponseWriter, codigo int, mensaje, detalle string) {
	escribirJSON(w, codigo, ErrorHTTP{Error: mensaje, Detalle: detalle})
}

// segmentoFinal extrae el ultimo segmento de una ruta ("/saldo/1234" -> "1234").
func segmentoFinal(ruta, prefijo string) string {
	resto := strings.TrimPrefix(ruta, prefijo)
	resto = strings.Trim(resto, "/")
	if i := strings.Index(resto, "/"); i >= 0 {
		resto = resto[:i]
	}
	return resto
}

// parametroEntero lee un parametro numerico de la query con valor por defecto.
func parametroEntero(r *http.Request, clave string, porDefecto int) int {
	if v := r.URL.Query().Get(clave); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return porDefecto
}

// ConCORS permite que la interfaz React (otro origen) consuma la API.
func ConCORS(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		siguiente.ServeHTTP(w, r)
	})
}
