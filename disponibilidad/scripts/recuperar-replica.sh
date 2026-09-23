#!/usr/bin/env bash
# Vuelve a levantar una replica caida para observar la transicion CAIDA -> VIVA.
# Uso: ./scripts/recuperar-replica.sh [A|B|C]
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

REPLICA="${1:-B}"
CONTENEDOR="recaudo-replica-$(echo "${REPLICA}" | tr '[:upper:]' '[:lower:]')"

echo "levantando ${CONTENEDOR}..."
docker start "${CONTENEDOR}"
sleep 3
echo "estado del dispatcher:"; curl -s ${URL_DISPATCHER}/estado; echo
echo "bitacora:"; tail -n 5 evidencia/monitor.log
