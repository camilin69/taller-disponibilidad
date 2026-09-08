// Comando replica: proceso servidor independiente que responde el eco del
// Ping/Echo y las consultas de saldo. Cada replica corre en su propio
// contenedor; no son objetos dentro de un mismo proceso.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/camilin69/taller-disponibilidad/internal/adapter/httpin"
	"github.com/camilin69/taller-disponibilidad/internal/infra/config"
	"github.com/camilin69/taller-disponibilidad/internal/infra/servidor"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	cfg, err := config.CargarReplica()
	if err != nil {
		log.Fatalf("configuracion invalida: %v", err)
	}
	log.SetPrefix("[replica " + cfg.ID + "] ")

	api := httpin.NuevaReplica(cfg.ID, cfg.SaldoBase, cfg.LatenciaMin, cfg.LatenciaMax)
	// En produccion el gancho de caos si termina el proceso.
	api.Salir = func(codigo int) {
		log.Printf("terminando el proceso por orden del inyector (codigo %d)", codigo)
		os.Exit(codigo)
	}

	ctx, detener := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer detener()

	log.Printf("saldo base=%d latencia simulada=[%v, %v]", cfg.SaldoBase, cfg.LatenciaMin, cfg.LatenciaMax)
	if err := servidor.Ejecutar(ctx, servidor.Opciones{
		Nombre:  "replica-" + cfg.ID,
		Puerto:  cfg.Puerto,
		Handler: api.Rutas(),
	}); err != nil {
		log.Fatalf("servidor: %v", err)
	}
	log.Println("apagado limpio")
}
