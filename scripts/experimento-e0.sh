#!/usr/bin/env bash
# Experimento E0 (linea base): carga constante sin fallas.
# Uso: ./scripts/experimento-e0.sh [duracion_s] [tasa_rps] [archivo_csv]
set -euo pipefail

# Git Bash (Windows) reescribe los argumentos que parecen rutas absolutas
# ("/datos/x" -> "C:/Program Files/Git/datos/x"). Se desactiva esa conversion
# para que las rutas dentro del contenedor lleguen intactas. En Linux/macOS
# estas variables simplemente se ignoran.
export MSYS_NO_PATHCONV=1
export MSYS2_ARG_CONV_EXCL='*'

cd "$(dirname "$0")/.."

# URL del dispatcher vista desde el host (configurable para evitar choques de puertos).
URL_DISPATCHER="${DISPATCHER_URL_PUBLICA:-http://localhost:8080}"

DURACION="${1:-40}"
TASA="${2:-20}"
CSV="${3:-e0_cliente.csv}"

echo "=== E0 (linea base): ${TASA} req/s durante ${DURACION} s ==="
echo "Estado inicial de las replicas:"
curl -s ${URL_DISPATCHER}/estado; echo

docker compose --profile carga run --rm \
  -e ETIQUETA=E0 \
  -e DURACION_S="${DURACION}" \
  -e TASA_RPS="${TASA}" \
  -e CSV_SALIDA="/datos/${CSV}" \
  cliente

echo
echo "Estado final de las replicas:"
curl -s ${URL_DISPATCHER}/estado; echo
echo "CSV generado en evidencia/${CSV}"
