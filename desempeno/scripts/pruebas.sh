#!/usr/bin/env bash
# Ejecuta todas las pruebas automatizadas (unitarias + integracion).
# Si Go no esta instalado en el equipo, usa la imagen oficial de Go en Docker.
#
# Uso: ./scripts/pruebas.sh [-race]
set -euo pipefail

export MSYS_NO_PATHCONV=1
export MSYS2_ARG_CONV_EXCL='*'

cd "$(dirname "$0")/.."

EXTRA="${1:-}"

if command -v go >/dev/null 2>&1; then
  echo "=== go vet ==="
  go vet ./...
  echo "=== pruebas unitarias y de integracion ==="
  go test ${EXTRA} ./... -count=1
else
  echo "Go no esta instalado: se usa la imagen golang:1.21 en Docker"
  # El detector de carreras necesita cgo, y la imagen alpine no trae compilador.
  PREPARAR=""
  if [ "${EXTRA}" = "-race" ]; then
    PREPARAR="apk add --no-cache gcc musl-dev >/dev/null && "
  fi
  docker run --rm -v "$(pwd):/src" -w /src golang:1.21-alpine \
    sh -c "${PREPARAR}go vet ./... && go test ${EXTRA} ./... -count=1"
fi
