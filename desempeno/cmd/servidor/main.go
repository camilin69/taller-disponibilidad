// Comando servidor: unico proceso que atiende el resumen mensual de viajes.
// Aplica las dos tacticas de desempeno del taller: un pool configurable de W
// trabajadores (Introducir Concurrencia) y un cache en memoria (Mantener
// Multiples Copias de Datos).
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/camilin69/taller-desempeno/internal/infra/config"
	"github.com/camilin69/taller-desempeno/internal/infra/ensamblaje"
	"github.com/camilin69/taller-desempeno/internal/infra/servidor"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetPrefix("[servidor] ")

	cfg, err := config.CargarServidor()
	if err != nil {
		log.Fatalf("configuracion invalida: %v", err)
	}

	sistema, err := ensamblaje.ArmarServidor(cfg)
	if err != nil {
		log.Fatalf("no se pudo armar el servidor: %v", err)
	}

	ctx, detener := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer detener()

	sistema.IniciarTrabajadores(ctx)

	log.Printf("R1 concurrencia: W=%d trabajadores | cola maxima=%d solicitudes",
		cfg.Trabajadores, cfg.CapacidadCola)
	log.Printf("R2 cache: %v", estadoCache(cfg.CacheActiva))
	log.Printf("calculo costoso: %v fijos por resumen (%d viajes por defecto)",
		cfg.CostoCalculo, cfg.ViajesPorDefecto)
	log.Printf("capacidad teorica: %.1f solicitudes/s (W / costo)", cfg.CapacidadTeorica())
	log.Printf("traza de peticiones=%v (trafico de inspeccion incluido=%v)",
		cfg.LogPeticiones, cfg.LogVigilancia)

	if err := servidor.Ejecutar(ctx, servidor.Opciones{
		Nombre:  "servidor",
		Puerto:  cfg.Puerto,
		Handler: sistema.API,
	}); err != nil {
		log.Fatalf("servidor: %v", err)
	}
	log.Println("apagado limpio")
}

func estadoCache(activa bool) string {
	if activa {
		return "ACTIVO (en memoria, sin expiracion)"
	}
	return "APAGADO (toda solicitud ejecuta el calculo)"
}
