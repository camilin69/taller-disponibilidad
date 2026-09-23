# --- Etapa 1: compilacion del generador de carga ---
FROM golang:1.21-alpine AS constructor
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/cliente ./cmd/cliente

# --- Etapa 2: imagen de ejecucion ---
FROM alpine:3.20
RUN apk add --no-cache tzdata
WORKDIR /app
COPY --from=constructor /bin/cliente /usr/local/bin/cliente
# /datos es el volumen donde queda el CSV con la evidencia del experimento.
RUN mkdir -p /datos
ENV CSV_SALIDA=/datos/cliente.csv
ENTRYPOINT ["/usr/local/bin/cliente"]
