// Package servidor encapsula el arranque y apagado ordenado de un servidor
// HTTP. Es infraestructura pura: no contiene reglas de negocio.
package servidor

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

// Opciones parametriza el servidor HTTP.
type Opciones struct {
	Nombre           string
	Puerto           int
	Handler          http.Handler
	TiempoApagado    time.Duration
	TiempoLecturaHdr time.Duration
}

// Ejecutar levanta el servidor y lo apaga ordenadamente cuando el contexto se
// cancela (Ctrl-C o SIGTERM de Docker). Bloquea hasta el apagado.
func Ejecutar(ctx context.Context, opts Opciones) error {
	if opts.TiempoApagado <= 0 {
		opts.TiempoApagado = 5 * time.Second
	}
	if opts.TiempoLecturaHdr <= 0 {
		opts.TiempoLecturaHdr = 5 * time.Second
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", opts.Puerto),
		Handler:           opts.Handler,
		ReadHeaderTimeout: opts.TiempoLecturaHdr,
		// Sin WriteTimeout: la carrera de redundancia ya esta acotada por su
		// propio context, y un WriteTimeout corto cortaria respuestas validas.
		IdleTimeout: 60 * time.Second,
	}

	errores := make(chan error, 1)
	go func() {
		log.Printf("[%s] escuchando en http://0.0.0.0:%d", opts.Nombre, opts.Puerto)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errores <- err
			return
		}
		errores <- nil
	}()

	select {
	case err := <-errores:
		return err
	case <-ctx.Done():
		log.Printf("[%s] apagando...", opts.Nombre)
		ctxApagado, cancelar := context.WithTimeout(context.Background(), opts.TiempoApagado)
		defer cancelar()
		if err := srv.Shutdown(ctxApagado); err != nil {
			return fmt.Errorf("apagado forzado de %s: %w", opts.Nombre, err)
		}
		return nil
	}
}
