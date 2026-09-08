# --- Etapa 1: compilacion del binario de la replica ---
FROM golang:1.21-alpine AS constructor
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/replica ./cmd/replica

# --- Etapa 2: imagen de ejecucion ---
FROM alpine:3.20
RUN apk add --no-cache tzdata
WORKDIR /app
COPY --from=constructor /bin/replica /usr/local/bin/replica
ENV PUERTO=8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/replica"]
