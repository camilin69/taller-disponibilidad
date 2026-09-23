#!/usr/bin/env bash
# Inyector de fallas: registra el timestamp y mata la replica indicada.
# Uso: ./scripts/inyectar-falla.sh [A|B|C] [kill|stop]
set -euo pipefail

# Git Bash (Windows) reescribe los argumentos que parecen rutas absolutas
# ("/datos/x" -> "C:/Program Files/Git/datos/x"). Se desactiva esa conversion
# para que las rutas dentro del contenedor lleguen intactas. En Linux/macOS
# estas variables simplemente se ignoran.
export MSYS_NO_PATHCONV=1
export MSYS2_ARG_CONV_EXCL='*'

cd "$(dirname "$0")/.."

REPLICA="${1:-B}"
COMANDO="${2:-kill}"
CONTENEDOR="recaudo-replica-$(echo "${REPLICA}" | tr '[:upper:]' '[:lower:]')"

docker compose --profile caos run --rm inyector \
  -objetivo "${CONTENEDOR}" -modo docker -comando "${COMANDO}"
