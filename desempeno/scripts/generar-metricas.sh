#!/usr/bin/env bash
# Calcula la tabla de metricas del taller (latencia, throughput, jitter,
# eventos no procesados y % desde cache) a partir de los CSV recolectados.
set -euo pipefail

export MSYS_NO_PATHCONV=1
export MSYS2_ARG_CONV_EXCL='*'

cd "$(dirname "$0")/.."

docker compose --profile reporte run --rm --build metricas \
  -csv "/datos/e0_cliente.csv,/datos/e1_cliente.csv,/datos/e2_run1_cliente.csv,/datos/e2_run2_cliente.csv" \
  -salida /datos/resultados.md

echo "reporte generado en evidencia/resultados.md"
