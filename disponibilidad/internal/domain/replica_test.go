package domain

import (
	"testing"
	"time"
)

func TestParsearEstado(t *testing.T) {
	casos := []struct {
		entrada  string
		esperado Estado
		conError bool
	}{
		{"VIVA", EstadoViva, false},
		{" viva ", EstadoViva, false},
		{"CAIDA", EstadoCaida, false},
		{"CAÍDA", EstadoCaida, false},
		{"zombie", "", true},
	}
	for _, c := range casos {
		got, err := ParsearEstado(c.entrada)
		if c.conError {
			if err == nil {
				t.Fatalf("ParsearEstado(%q): se esperaba error", c.entrada)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParsearEstado(%q): error inesperado %v", c.entrada, err)
		}
		if got != c.esperado {
			t.Fatalf("ParsearEstado(%q)=%q, se esperaba %q", c.entrada, got, c.esperado)
		}
	}
}

func TestReplicaValidaYURLs(t *testing.T) {
	r := Replica{ID: "A", URLBase: "http://replica-a:8080/"}
	if err := r.Valida(); err != nil {
		t.Fatalf("replica valida rechazada: %v", err)
	}
	if r.URLPing() != "http://replica-a:8080/ping" {
		t.Fatalf("URLPing incorrecta: %s", r.URLPing())
	}
	if r.URLSaldo("1234") != "http://replica-a:8080/saldo/1234" {
		t.Fatalf("URLSaldo incorrecta: %s", r.URLSaldo("1234"))
	}
	if err := (Replica{ID: "", URLBase: "http://x"}).Valida(); err == nil {
		t.Fatal("se esperaba error por id vacio")
	}
	if err := (Replica{ID: "A", URLBase: "replica-a:8080"}).Valida(); err == nil {
		t.Fatal("se esperaba error por esquema faltante")
	}
}

func TestCambioEstadoLinea(t *testing.T) {
	momento := time.Date(2026, 9, 7, 15, 4, 5, 123000000, time.UTC)
	c := CambioEstado{Momento: momento, ReplicaID: "B", Anterior: EstadoViva, Nuevo: EstadoCaida, Motivo: "2 fallos consecutivos"}
	esperado := "[2026-09-07T15:04:05.123Z] REPLICA_B VIVA -> CAIDA (2 fallos consecutivos)"
	if c.Linea() != esperado {
		t.Fatalf("linea=%q, se esperaba %q", c.Linea(), esperado)
	}
}

func TestSaldoEsValida(t *testing.T) {
	if !(Saldo{ReplicaID: "A", Valor: 15000}).EsValida() {
		t.Fatal("saldo valido rechazado")
	}
	if (Saldo{ReplicaID: "", Valor: 10}).EsValida() {
		t.Fatal("saldo sin replica_id deberia ser invalido")
	}
	if (Saldo{ReplicaID: "A", Valor: -1}).EsValida() {
		t.Fatal("saldo negativo deberia ser invalido")
	}
}

func TestValidarIDTarjeta(t *testing.T) {
	if err := ValidarIDTarjeta("1234"); err != nil {
		t.Fatalf("id valido rechazado: %v", err)
	}
	if err := ValidarIDTarjeta("  "); err == nil {
		t.Fatal("se esperaba error con id vacio")
	}
	if err := ValidarIDTarjeta("12/34"); err == nil {
		t.Fatal("se esperaba error con separador de ruta")
	}
}
