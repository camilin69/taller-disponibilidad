#!/usr/bin/env bash
# Experimento E1 (caida abrupta): carga constante + inyeccion de falla.
# Uso: ./scripts/experimento-e1.sh [replica] [corrida] [duracion_s] [tasa_rps] [segundo_inyeccion]
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
CORRIDA="${2:-run1}"
DURACION="${3:-40}"
TASA="${4:-20}"
SEGUNDO="${5:-15}"

CSV="e1_${CORRIDA}_cliente.csv"
CONTENEDOR="recaudo-replica-$(echo "${REPLICA}" | tr '[:upper:]' '[:lower:]')"

echo "=== E1 (${CORRIDA}): ${TASA} req/s durante ${DURACION} s, se mata ${CONTENEDOR} en t+${SEGUNDO} s ==="
echo "Estado inicial:"; curl -s ${URL_DISPATCHER}/estado; echo

# El cliente corre en segundo plano mientras el inyector actua.
docker compose --profile carga run --rm \
  -e ETIQUETA=E1 \
  -e DURACION_S="${DURACION}" \
  -e TASA_RPS="${TASA}" \
  -e CSV_SALIDA="/datos/${CSV}" \
  cliente &
PID_CLIENTE=$!

sleep "${SEGUNDO}"
echo "--- Inyectando la falla en ${CONTENEDOR} ---"
docker compose --profile caos run --rm inyector -objetivo "${CONTENEDOR}" -modo docker -comando kill

sleep 5
echo; echo "Estado tras la caida:"; curl -s ${URL_DISPATCHER}/estado; echo

echo "Esperando a que el cliente termine la carga..."
wait "${PID_CLIENTE}"

echo; echo "--- Bitacora del monitor ---"; tail -n 10 evidencia/monitor.log
echo; echo "--- Log del inyector ---"; tail -n 4 evidencia/inyector.log
echo; echo "Evidencia: evidencia/${CSV}, evidencia/monitor.log, evidencia/inyector.log"
echo "Para volver a levantar la replica: ./scripts/recuperar-replica.sh ${REPLICA}"
