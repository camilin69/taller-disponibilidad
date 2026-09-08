# --- Etapa 1: compilacion del binario del dispatcher ---
FROM golang:1.21-alpine AS constructor
WORKDIR /src
# El proyecto solo usa la biblioteca estandar; go.mod se copia primero para
# aprovechar la cache de capas de Docker.
COPY go.mod ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/dispatcher ./cmd/dispatcher

# --- Etapa 2: imagen de ejecucion ---
FROM alpine:3.20
RUN apk add --no-cache tzdata ca-certificates
WORKDIR /app
COPY --from=constructor /bin/dispatcher /usr/local/bin/dispatcher
# /datos es el volumen donde queda la bitacora del monitor (monitor.log).
RUN mkdir -p /datos
ENV PUERTO=8080 BITACORA_ARCHIVO=/datos/monitor.log
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/dispatcher"]
