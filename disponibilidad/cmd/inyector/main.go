// Comando inyector: script de inyeccion de fallas. Registra su propio
// timestamp JUSTO ANTES de matar la replica (ese timestamp es el punto de
// partida para medir el tiempo de deteccion) y luego termina el proceso
// objetivo con "docker kill" / "docker stop", o con POST /chaos/crash.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/camilin69/taller-disponibilidad/internal/domain"
	"github.com/camilin69/taller-disponibilidad/internal/infra/config"
)

func main() {
	cfg := config.CargarInyector()

	objetivo := flag.String("objetivo", cfg.Objetivo, "contenedor a matar (modo docker) o id de replica (modo http)")
	modo := flag.String("modo", cfg.Modo, "docker | http")
	comando := flag.String("comando", cfg.ComandoDocker, "kill | stop (solo modo docker)")
	urlReplica := flag.String("url", cfg.URLReplica, "URL base de la replica (solo modo http)")
	archivoLog := flag.String("log", cfg.ArchivoLog, "archivo donde se registra el timestamp de la inyeccion")
	flag.Parse()

	if strings.TrimSpace(*objetivo) == "" && strings.TrimSpace(*urlReplica) == "" {
		fmt.Fprintln(os.Stderr, "uso: inyector -objetivo <contenedor> [-modo docker|http] [-comando kill|stop] [-url http://replica-b:8080]")
		os.Exit(2)
	}

	// 1) El timestamp se registra ANTES de la falla: es obligatorio y es el
	//    origen del calculo "tiempo de deteccion".
	momento := time.Now()
	linea := fmt.Sprintf("[%s] INYECTOR objetivo=%s modo=%s comando=%s",
		momento.UTC().Format(domain.FormatoTimestamp), *objetivo, *modo, *comando)
	fmt.Println(linea)
	if err := anexar(*archivoLog, linea); err != nil {
		fmt.Fprintf(os.Stderr, "aviso: no se pudo escribir %s: %v\n", *archivoLog, err)
	}

	// 2) Se provoca la falla.
	var err error
	switch strings.ToLower(*modo) {
	case "docker":
		err = matarContenedor(*objetivo, *comando)
	case "http":
		err = crashPorHTTP(*urlReplica, *objetivo)
	default:
		err = fmt.Errorf("modo desconocido %q (use docker o http)", *modo)
	}

	fin := time.Now()
	resultado := "OK"
	if err != nil {
		resultado = "ERROR: " + err.Error()
	}
	cierre := fmt.Sprintf("[%s] INYECTOR resultado=%s duracion_ms=%d",
		fin.UTC().Format(domain.FormatoTimestamp), resultado, fin.Sub(momento).Milliseconds())
	fmt.Println(cierre)
	if errLog := anexar(*archivoLog, cierre); errLog != nil {
		fmt.Fprintf(os.Stderr, "aviso: no se pudo escribir %s: %v\n", *archivoLog, errLog)
	}
	if err != nil {
		os.Exit(1)
	}
}

// matarContenedor ejecuta docker kill|stop sobre el contenedor objetivo.
func matarContenedor(contenedor, comando string) error {
	if comando != "kill" && comando != "stop" {
		return fmt.Errorf("comando docker no permitido: %q", comando)
	}
	cmd := exec.Command("docker", comando, contenedor)
	var salida bytes.Buffer
	cmd.Stdout = &salida
	cmd.Stderr = &salida
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker %s %s: %v (%s)", comando, contenedor, err, strings.TrimSpace(salida.String()))
	}
	return nil
}

// crashPorHTTP usa el gancho de caos de la replica (POST /chaos/crash).
func crashPorHTTP(urlBase, objetivo string) error {
	if strings.TrimSpace(urlBase) == "" {
		return fmt.Errorf("en modo http se requiere -url (ej: http://localhost:8082)")
	}
	url := strings.TrimRight(urlBase, "/") + "/chaos/crash"
	cliente := &http.Client{Timeout: 2 * time.Second}
	respuesta, err := cliente.Post(url, "application/json", nil)
	if err != nil {
		return fmt.Errorf("POST %s: %w", url, err)
	}
	defer func() { _ = respuesta.Body.Close() }()
	cuerpo, _ := io.ReadAll(io.LimitReader(respuesta.Body, 4096))
	if respuesta.StatusCode != http.StatusOK {
		return fmt.Errorf("la replica %s respondio %d: %s", objetivo, respuesta.StatusCode, strings.TrimSpace(string(cuerpo)))
	}
	return nil
}

// anexar agrega una linea al archivo de evidencia del inyector.
func anexar(ruta, linea string) error {
	if strings.TrimSpace(ruta) == "" {
		return nil
	}
	if dir := filepath.Dir(ruta); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(ruta, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(linea + "\n"); err != nil {
		return err
	}
	return f.Sync()
}
