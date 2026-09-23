# --- Etapa 1: compilacion del inyector de fallas ---
FROM golang:1.21-alpine AS constructor
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/inyector ./cmd/inyector

# --- Etapa 2: imagen de ejecucion ---
# Incluye el cliente de linea de comandos de Docker para poder ejecutar
# "docker kill" sobre el contenedor de la replica objetivo. Requiere montar
# el socket del demonio: -v /var/run/docker.sock:/var/run/docker.sock
FROM alpine:3.20
RUN apk add --no-cache docker-cli tzdata
WORKDIR /app
COPY --from=constructor /bin/inyector /usr/local/bin/inyector
RUN mkdir -p /datos
ENV INYECTOR_LOG=/datos/inyector.log MODO=docker
ENTRYPOINT ["/usr/local/bin/inyector"]
