// Package gateway contiene los adaptadores de salida hacia las replicas.
// Aqui vive todo el detalle HTTP; los casos de uso solo conocen los puertos.
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
)

// respuestaPing es el cuerpo esperado del eco: {"replica_id":"A"}.
type respuestaPing struct {
	ReplicaID string `json:"replica_id"`
}

// SondeadorHTTP implementa el puerto usecase.Sondeador con HTTP/1.1.
// El tiempo limite t no se fija aqui: llega en el context del caso de uso, de
// modo que el parametro sigue siendo configurable desde el entorno.
type SondeadorHTTP struct {
	cliente *http.Client
}

// NuevoSondeadorHTTP construye el adaptador con un transporte propio, separado
// del de las consultas de saldo: asi una tormenta de consultas no consume las
// conexiones del monitor (aislamiento de recursos).
func NuevoSondeadorHTTP(maxConexionesPorHost int) *SondeadorHTTP {
	if maxConexionesPorHost <= 0 {
		maxConexionesPorHost = 4
	}
	transporte := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 500 * time.Millisecond, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:        maxConexionesPorHost * 8,
		MaxIdleConnsPerHost: maxConexionesPorHost,
		MaxConnsPerHost:     maxConexionesPorHost * 2,
		IdleConnTimeout:     30 * time.Second,
	}
	return &SondeadorHTTP{cliente: &http.Client{Transport: transporte}}
}

// Sondear envia GET /ping y valida el eco. Cualquier error de red, codigo
// distinto de 200, cuerpo ilegible o eco con identificador ajeno cuenta como
// fallo de sondeo.
func (s *SondeadorHTTP) Sondear(ctx context.Context, r domain.Replica) error {
	peticion, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URLPing(), nil)
	if err != nil {
		return fmt.Errorf("no se pudo construir el ping a %s: %w", r.ID, err)
	}
	peticion.Header.Set("Accept", "application/json")

	respuesta, err := s.cliente.Do(peticion)
	if err != nil {
		return fmt.Errorf("ping a %s fallido: %w", r.ID, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(respuesta.Body, 4096))
		_ = respuesta.Body.Close()
	}()

	if respuesta.StatusCode != http.StatusOK {
		return fmt.Errorf("ping a %s respondio %d", r.ID, respuesta.StatusCode)
	}

	var cuerpo respuestaPing
	if err := json.NewDecoder(io.LimitReader(respuesta.Body, 4096)).Decode(&cuerpo); err != nil {
		return fmt.Errorf("eco ilegible de %s: %w", r.ID, err)
	}
	// Ping/ECHO: el eco debe identificar a la replica que responde.
	if !strings.EqualFold(strings.TrimSpace(cuerpo.ReplicaID), r.ID) {
		return fmt.Errorf("eco de %s con identificador inesperado %q", r.ID, cuerpo.ReplicaID)
	}
	return nil
}
