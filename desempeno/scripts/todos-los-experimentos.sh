#!/usr/bin/env bash
# Ejecuta el protocolo experimental completo del taller: E0, E1 y E2 (dos
# corridas de E2, como pide el enunciado) y genera la tabla de metricas.
set -euo pipefail

cd "$(dirname "$0")/.."

./scripts/experimento.sh E0
./scripts/experimento.sh E1
# E2 se ejecuta dos veces para confirmar los valores (numeral 7 del enunciado).
./scripts/experimento.sh E2 run1
./scripts/experimento.sh E2 run2

./scripts/generar-metricas.sh
