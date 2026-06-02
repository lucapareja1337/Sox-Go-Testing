# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Estágio 1 — builder: compila os dois binários estáticos
# ---------------------------------------------------------------------------
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache ca-certificates

WORKDIR /src

# Camada de cache de dependências: copia só os manifests antes do código.
# Enquanto go.mod/go.sum não mudarem, esta camada é reaproveitada.
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Código-fonte
COPY . .

# lib/pq é Go puro → CGO desligado gera binário 100% estático.
# -trimpath remove caminhos absolutos; -s -w removem tabela de símbolos/debug.
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/migrate   ./cmd/migrate \
 && go build -trimpath -ldflags="-s -w" -o /out/simulador ./cmd/simulador

# ---------------------------------------------------------------------------
# Estágio 2 — runtime: imagem mínima só com os binários e as migrações
# ---------------------------------------------------------------------------
FROM alpine:3.20

# ca-certificates p/ TLS; tzdata p/ as regras de horário (R3) baterem com
# o fuso configurado via TZ; usuário não-root por princípio de menor privilégio.
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 appuser

WORKDIR /app

# Binários
COPY --from=builder /out/migrate   /app/migrate
COPY --from=builder /out/simulador /app/simulador

# O migrador lê "./migrations" relativo ao WORKDIR — por isso copiamos para /app.
COPY migrations ./migrations

USER appuser

# Sem ENTRYPOINT fixo: cada serviço/comando escolhe o binário.
# Default inofensivo: mostra a versão das migrações aplicadas.
CMD ["/app/migrate", "version"]