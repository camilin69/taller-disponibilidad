# Herramienta de analisis de evidencia (CSV + bitacora + log del inyector).
FROM golang:1.21-alpine AS constructor
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/metricas ./cmd/metricas

FROM alpine:3.20
WORKDIR /datos
COPY --from=constructor /bin/metricas /usr/local/bin/metricas
ENTRYPOINT ["/usr/local/bin/metricas"]
