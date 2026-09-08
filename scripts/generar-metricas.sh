#!/usr/bin/env bash
# Calcula la tabla de metricas del taller a partir de la evidencia recolectada.
# Uso: ./scripts/generar-metricas.sh
set -euo pipefail

# Git Bash (Windows) reescribe los argumentos que parecen rutas absolutas
# ("/datos/x" -> "C:/Program Files/Git/datos/x"). Se desactiva esa conversion
# para que las rutas dentro del contenedor lleguen intactas. En Linux/macOS
# estas variables simplemente se ignoran.
export MSYS_NO_PATHCONV=1
export MSYS2_ARG_CONV_EXCL='*'

cd "$(dirname "$0")/.."

docker compose --profile reporte run --rm metricas \
  -csv "/datos/e0_cliente.csv,/datos/e1_run1_cliente.csv,/datos/e1_run2_cliente.csv" \
  -bitacora /datos/monitor.log \
  -inyector /datos/inyector.log \
  -salida /datos/resultados.md

echo "reporte generado en evidencia/resultados.md"
