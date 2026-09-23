// Comando dispatcher: unico punto de entrada del cliente. Ejecuta al mismo
// tiempo, y sin que una tarea bloquee a la otra:
//   - el monitor Ping/Echo (R1), una goroutine por replica;
//   - la atencion de consultas con redundancia activa (R2).
//
// El cableado concreto vive en internal/infra/ensamblaje, de modo que las
// pruebas de integracion levanten exactamente el mismo sistema.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/camilin69/taller-disponibilidad/internal/infra/config"
	"github.com/camilin69/taller-disponibilidad/internal/infra/ensamblaje"
	"github.com/camilin69/taller-disponibilidad/internal/infra/servidor"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetPrefix("[dispatcher] ")

	cfg, err := config.CargarDispatcher()
	if err != nil {
		log.Fatalf("configuracion invalida: %v", err)
	}

	sistema, err := ensamblaje.ArmarDispatcher(cfg)
	if err != nil {
		log.Fatalf("no se pudo armar el dispatcher: %v", err)
	}
	defer func() { _ = sistema.Cerrar() }()

	ctx, detener := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer detener()

	log.Printf("replicas configuradas: %v", cfg.Replicas)
	log.Printf("monitor Ping/Echo: T=%v t=%v k=%d m=%d (deteccion esperada ~%v)",
		cfg.IntervaloSondeo, cfg.TimeoutSondeo, cfg.K, cfg.M, cfg.DeteccionEsperada())
	log.Printf("bitacora: %s", cfg.ArchivoBitacora)
	log.Printf("traza de peticiones=%v (trafico de vigilancia incluido=%v)", cfg.LogPeticiones, cfg.LogVigilancia)

	// El monitor corre en segundo plano: atender consultas nunca espera un ciclo
	// de sondeo, ni el sondeo se detiene mientras se atienden consultas.
	sistema.IniciarMonitor(ctx)

	if err := servidor.Ejecutar(ctx, servidor.Opciones{
		Nombre:  "dispatcher",
		Puerto:  cfg.Puerto,
		Handler: sistema.API,
	}); err != nil {
		log.Fatalf("servidor: %v", err)
	}

	sistema.Monitor.Esperar()
	log.Println("apagado limpio")
}
