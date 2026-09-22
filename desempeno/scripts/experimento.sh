#!/usr/bin/env bash
# Ejecuta un experimento del taller de desempeno (E0, E1 o E2).
#
# Recrea el servidor con la configuracion del experimento (W y cache), espera a
# que responda, dispara la rafaga y deja el CSV en evidencia/.
#
# Recrear el servidor en cada corrida no es opcional: es lo que garantiza que
# el cache arranque vacio y que W sea el del experimento. Sin eso, E2 heredaria
# el cache de la corrida anterior y sus numeros no valdrian.
#
# Uso: ./scripts/experimento.sh E0|E1|E2 [corrida] [duracion_s] [tasa_rps] [tarjetas]
set -euo pipefail

# Git Bash (Windows) reescribe los argumentos que parecen rutas absolutas
# ("/datos/x" -> "C:/Program Files/Git/datos/x"). Se desactiva esa conversion
# para que las rutas dentro del contenedor lleguen intactas. En Linux/macOS
# estas variables simplemente se ignoran.
export MSYS_NO_PATHCONV=1
export MSYS2_ARG_CONV_EXCL='*'

cd "$(dirname "$0")/.."

EXPERIMENTO="${1:-}"
CORRIDA="${2:-}"
DURACION="${3:-16}"
TASA="${4:-8}"
TARJETAS="${5:-10}"

case "${EXPERIMENTO}" in
  E0) TRABAJADORES=1; CACHE=false; DESCRIPCION="linea base: un solo trabajador, sin cache" ;;
  E1) TRABAJADORES=8; CACHE=false; DESCRIPCION="con concurrencia: W=8, sin cache" ;;
  E2) TRABAJADORES=8; CACHE=true;  DESCRIPCION="con concurrencia y cache: W=8, cache activo" ;;
  *)  echo "uso: $0 E0|E1|E2 [corrida] [duracion_s] [tasa_rps] [tarjetas]" >&2; exit 1 ;;
esac

SUFIJO=""
ETIQUETA="${EXPERIMENTO}"
if [ -n "${CORRIDA}" ]; then
  SUFIJO="_${CORRIDA}"
  ETIQUETA="${EXPERIMENTO}-${CORRIDA}"
fi
CSV="$(echo "${EXPERIMENTO}" | tr '[:upper:]' '[:lower:]')${SUFIJO}_cliente.csv"

URL_SERVIDOR="${SERVIDOR_URL_PUBLICA:-http://localhost:8090}"

echo "=== ${EXPERIMENTO} (${DESCRIPCION}) ==="
echo "rafaga: ${TASA} req/s durante ${DURACION} s = $((TASA * DURACION)) solicitudes sobre ${TARJETAS} tarjetas"

# Las trazas se apagan durante la carga: escribir una linea por solicitud
# mientras se mide la latencia contamina justo lo que se quiere medir.
echo "recreando el servidor con W=${TRABAJADORES} cache=${CACHE}..."
TRABAJADORES="${TRABAJADORES}" CACHE_ACTIVA="${CACHE}" LOG_PETICIONES=false \
  docker compose up -d --build --force-recreate servidor

# Esperar a que el servidor responda antes de disparar la rafaga.
for _ in $(seq 1 60); do
  if curl -sf "${URL_SERVIDOR}/salud" >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done
if ! curl -sf "${URL_SERVIDOR}/salud" >/dev/null 2>&1; then
  echo "el servidor no respondio en ${URL_SERVIDOR}" >&2
  exit 1
fi

echo "configuracion en vivo:"
curl -s "${URL_SERVIDOR}/config"; echo

# --build evita que compose intente PULL de una imagen que solo existe local
# ("pull access denied") antes de decidirse a construirla.
docker compose --profile carga run --rm --build \
  -e ETIQUETA="${ETIQUETA}" \
  -e DURACION_S="${DURACION}" \
  -e TASA_RPS="${TASA}" \
  -e NUM_TARJETAS="${TARJETAS}" \
  -e CSV_SALIDA="/datos/${CSV}" \
  cliente

echo
echo "configuracion final (trabajadores ocupados y entradas del cache):"
curl -s "${URL_SERVIDOR}/config"; echo
echo "CSV generado en evidencia/${CSV}"
