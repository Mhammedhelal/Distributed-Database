# ── Stage 1: build ────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /src

# Cache module downloads separately from source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build a statically linked binary so the runtime image needs no libc.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /gateway ./cmd/gateway

# ── Stage 2: runtime ──────────────────────────────────────────────────────────
FROM scratch

# Copy CA certificates for outbound HTTPS (health probes, upstream nodes).
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary and default config.
COPY --from=builder /gateway /gateway
COPY --from=builder /src/config/gateway.yaml /config/gateway.yaml

EXPOSE 8000

ENTRYPOINT ["/gateway", "-config", "/config/gateway.yaml"]