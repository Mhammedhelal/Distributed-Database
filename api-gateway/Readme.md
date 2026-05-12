# api-gateway

The API Gateway is the **single external entry point** for the distributed database cluster. No client or GUI should ever talk directly to a master or worker node.

## Responsibilities

| Concern | Implementation |
|---|---|
| Security — strip spoofed master token | `StripExternalMasterToken` middleware (runs first) |
| Rate limiting | Per-IP token-bucket (`internal/ratelimit`) |
| Client auth | Optional bearer-token check (`internal/auth`) |
| Write routing | `POST /query` → master |
| Read routing | `GET /query` → round-robin worker |
| Admin routing | `POST /admin/*` → master only |
| Replication blocking | `POST /replication/*` → 403 from outside |
| Inter-node auth | HMAC-SHA256 `X-Master-Token` injected on slave calls |
| Cluster health | `GET /cluster/status` aggregates all node `/health` endpoints |

## Running locally

```bash
# Copy and edit the config:
cp config/gateway.yaml config/gateway.local.yaml

# Set the HMAC secret (never commit a real secret):
export GATEWAY_HMAC_SECRET="super-secret-32-char-minimum"

# Run directly:
go run ./cmd/gateway -config config/gateway.local.yaml

# Or via Docker Compose (from repo root):
docker compose up api-gateway
```

## Configuration

All values can be overridden with environment variables:

| Env var | Config key | Description |
|---|---|---|
| `GATEWAY_HMAC_SECRET` | `auth.hmac_secret` | Shared HMAC secret with all nodes — **required** |
| `GATEWAY_CLIENT_API_KEY` | `auth.client_api_key` | Bearer token for external clients (empty = disabled) |
| `MASTER_ADDRESS` | `nodes.master.address` | Master node HTTP address |

## Request flow

```
External client
      │
      ▼
[1] StripExternalMasterToken   ← removes any client-supplied X-Master-Token
      │
      ▼
[2] RateLimit                  ← 429 if per-IP bucket exhausted
      │
      ▼
[3] ClientAuth (optional)      ← 401 if bearer token missing/invalid
      │
      ▼
[4] Router                     ← decides destination
      │
      ├── POST /query          → master
      ├── GET  /query          → worker (round-robin) + X-Consistency-Level: eventual
      ├── POST /admin/*        → master
      ├── POST /replication/*  → 403 FORBIDDEN
      ├── POST /analytics      → worker-1 (C++) + inject X-Master-Token
      ├── POST /search         → worker-2 (Python)
      └── GET  /cluster/status → health proxy (no upstream)
```

## Security invariant

`StripExternalMasterToken` runs **before** the router and before client auth. This means:

- A client that knows the HMAC secret and includes `X-Master-Token` in their request will have it silently removed before it reaches any upstream.
- The token is **only ever added** by `InjectMasterToken` in the router, when the gateway itself forwards an authorised call to a slave.
- Workers that receive a write without a valid `X-Master-Token` return **403 Forbidden**.

## Running tests

```bash
go test ./... -v
```

Tests use `httptest.Server` — no live nodes required.