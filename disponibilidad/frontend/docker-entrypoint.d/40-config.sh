#!/bin/sh
# Genera la configuracion en tiempo de arranque: la URL del dispatcher es una
# variable de entorno, no un valor compilado en el bundle.
set -e
: "${DISPATCHER_URL:=http://localhost:8080}"
cat > /usr/share/nginx/html/config.js <<EOF
window.__CONFIG__ = { DISPATCHER_URL: "${DISPATCHER_URL}" };
EOF
echo "[frontend] DISPATCHER_URL=${DISPATCHER_URL}"
