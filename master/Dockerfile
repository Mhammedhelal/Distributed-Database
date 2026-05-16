FROM golang:1.22-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /master ./cmd/server

# Use alpine instead of scratch for the runtime image
FROM alpine:latest

WORKDIR /app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /master /master
COPY --from=builder /src/config/master.json /config/master.json

RUN mkdir -p /data

EXPOSE 8080

CMD ["/master"]