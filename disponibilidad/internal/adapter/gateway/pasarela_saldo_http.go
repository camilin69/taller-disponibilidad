package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
)

// PasarelaSaldoHTTP implementa el puerto usecase.PasarelaSaldo contra el
// endpoint GET /saldo/{idTarjeta} de una replica.
type PasarelaSaldoHTTP struct {
	cliente *http.Client
}

// NuevaPasarelaSaldoHTTP construye el adaptador de consultas. El pool se
// dimensiona para soportar el fan-out de la redundancia activa a 20 req/s.
func NuevaPasarelaSaldoHTTP(maxConexionesPorHost int) *PasarelaSaldoHTTP {
	if maxConexionesPorHost <= 0 {
		maxConexionesPorHost = 64
	}
	transporte := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:        maxConexionesPorHost * 4,
		MaxIdleConnsPerHost: maxConexionesPorHost,
		MaxConnsPerHost:     maxConexionesPorHost * 2,
		IdleConnTimeout:     60 * time.Second,
	}
	return &PasarelaSaldoHTTP{cliente: &http.Client{Transport: transporte}}
}

// ConsultarSaldo pide el saldo a UNA replica. Solo un 200 con cuerpo valido se
// considera respuesta valida; el resto es un fallo que descarta al competidor.
func (p *PasarelaSaldoHTTP) ConsultarSaldo(ctx context.Context, r domain.Replica, idTarjeta string) (domain.Saldo, error) {
	peticion, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URLSaldo(idTarjeta), nil)
	if err != nil {
		return domain.Saldo{}, fmt.Errorf("no se pudo construir la consulta a %s: %w", r.ID, err)
	}
	peticion.Header.Set("Accept", "application/json")

	respuesta, err := p.cliente.Do(peticion)
	if err != nil {
		return domain.Saldo{}, fmt.Errorf("consulta a %s fallida: %w", r.ID, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(respuesta.Body, 8192))
		_ = respuesta.Body.Close()
	}()

	if respuesta.StatusCode != http.StatusOK {
		return domain.Saldo{}, fmt.Errorf("consulta a %s respondio %d", r.ID, respuesta.StatusCode)
	}

	var saldo domain.Saldo
	if err := json.NewDecoder(io.LimitReader(respuesta.Body, 8192)).Decode(&saldo); err != nil {
		return domain.Saldo{}, fmt.Errorf("cuerpo ilegible de %s: %w", r.ID, err)
	}
	if saldo.ReplicaID == "" {
		saldo.ReplicaID = r.ID
	}
	if saldo.IDTarjeta == "" {
		saldo.IDTarjeta = idTarjeta
	}
	return saldo, nil
}
