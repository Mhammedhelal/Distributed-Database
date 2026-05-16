FROM golang:1.22-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /gateway ./cmd/gateway

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /gateway /gateway
COPY --from=builder /src/config/gateway.json /config/gateway.json

EXPOSE 8000

ENTRYPOINT ["/gateway", "-config", "/config/gateway.json"]