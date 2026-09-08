#!/usr/bin/env bash
# Ejecuta todas las pruebas automatizadas (unitarias + integracion).
# Uso: ./scripts/pruebas.sh [-race]
set -euo pipefail

# Git Bash (Windows) reescribe los argumentos que parecen rutas absolutas
# ("/datos/x" -> "C:/Program Files/Git/datos/x"). Se desactiva esa conversion
# para que las rutas dentro del contenedor lleguen intactas. En Linux/macOS
# estas variables simplemente se ignoran.
export MSYS_NO_PATHCONV=1
export MSYS2_ARG_CONV_EXCL='*'

cd "$(dirname "$0")/.."

EXTRA="${1:-}"
echo "=== go vet ==="
go vet ./...
echo "=== pruebas unitarias y de integracion ${EXTRA} ==="
go test ${EXTRA} ./... -count=1
