# Distributed Database

A fully distributed, fault-tolerant database system with master-worker replication, automatic failover via Bully election, and a web-based query interface.

**Components:**

- **API Gateway** (Go) — Routes requests, enforces security, rate-limits
- **Master** (Go) — Writes, replication coordinator, leader election
- **Worker 1** (C++) — Read replica, query execution
- **Worker 2** (Python) — Read replica, query execution  
- **GUI** (Streamlit) — Web interface for queries, tables, cluster health

---

## Quick Start — Get Running in 5 Minutes

### Prerequisites

Install these on your machine:

- **Docker Desktop** (includes Docker Compose) — [docker.com](https://docker.com)
- **Git** (optional, to clone)

That's it. Everything else runs inside containers.

---

### 1 — Set up the project

Put all the delivered folders into one directory:

```
distributed-db/
├── api-gateway/
├── master/
├── worker-node1/
├── worker-node2/
├── gui/
├── scripts/
└── docker-compose.yml
```

---

### 2 — Configure the secret

Create a `.env` file in the root (next to `docker-compose.yml`):

```bash
HMAC_SECRET=my-super-secret-32-char-minimum!!
```

This shared secret is used by every node to sign and verify `X-Master-Token`. Never commit it.

---

### 3 — Download Crow (Worker 1 only)

Worker 1 is C++ and needs the Crow single-header file:

```bash
curl -L https://github.com/CrowCpp/Crow/releases/download/v1.1.0/crow_all.h \
     -o worker-node1/include/crow_all.h
```

---

### 4 — Start the cluster

```bash
docker compose up --build
```

First build takes ~3–5 minutes (compiling C++). Subsequent starts are fast.

Wait until you see all services healthy — you'll see log lines like:

```
master      | {"level":"INFO","msg":"master listening","addr":":8080"}
worker1     | worker-node1 (C++) listening on :8081
worker2     | INFO:     Application startup complete.
api-gateway | {"level":"INFO","msg":"gateway listening","addr":":8000"}
```

---

### 5 — Open the GUI

Visit **<http://localhost:8501>** in your browser.

```
Query Editor  →  run SQL
Table Browser →  browse tables and data
Cluster Health → see live node status
Admin         →  create/drop databases
```

---

### 6 — Seed with demo data

In a new terminal:

```bash
GATEWAY_URL=http://localhost:8000 ./scripts/seed.sh
```

This creates a `distdb` database with `users`, `products`, and `orders` tables and inserts sample rows. After seeding, go to **Table Browser** in the GUI and type `distdb` to explore.

---

## Usage Examples

### Run the smoke tests

```bash
GATEWAY_URL=http://localhost:8000 ./scripts/test_cluster.sh
```

Runs 12 automated checks: health, security (replication blocked from outside, writes blocked on slave without token), full CRUD lifecycle, replication ACKs, and DROP DATABASE.

---

### Try it manually with curl

```bash
# Create a database
curl -X POST http://localhost:8000/query \
  -H "Content-Type: application/json" \
  -d '{"sql": "CREATE DATABASE mydb"}'

# Create a table
curl -X POST http://localhost:8000/query \
  -H "Content-Type: application/json" \
  -d '{"sql": "CREATE TABLE users (name VARCHAR(100), email VARCHAR(255), age INT)"}'

# Insert a row  →  goes to master, replicated to both workers
curl -X POST http://localhost:8000/query \
  -H "Content-Type: application/json" \
  -d '{"sql": "INSERT INTO users (name, email, age) VALUES ('"'"'Alice'"'"', '"'"'alice@example.com'"'"', 30)"}'

# Read  →  load-balanced across workers
curl -X POST http://localhost:8000/query \
  -H "Content-Type: application/json" \
  -d '{"sql": "SELECT * FROM users"}'

# Cluster health
curl http://localhost:8000/cluster/status
```

---

### Failover demo

See the Bully election in action:

```bash
./scripts/failover_demo.sh
```

This stops the master container, waits 15 seconds for a worker to be elected, verifies reads still work, then restarts the master and confirms it rejoins.

---

## Service ports

| Service | Port | URL |
|---|---|---|
| API Gateway | 8000 | `http://localhost:8000` |
| Master (Go) | 8080 | `http://localhost:8080` |
| Worker 1 (C++) | 8081 | `http://localhost:8081` |
| Worker 2 (Python) | 8082 | `http://localhost:8082` |
| Streamlit GUI | 8501 | `http://localhost:8501` |

---

## Stop the cluster

```bash
docker compose down          # stop but keep data
docker compose down -v       # stop and delete all MySQL data (clean slate)
```

---

## Architecture

```
                    ┌─────────────────────────┐
                    │   API Gateway (Go)      │
                    │  - Auth enforcement     │
                    │  - Rate limiting        │
                    │  - Request routing      │
                    └────────────┬────────────┘
                                 │
                    ┌────────────┴────────────┐
                    │ (route based on op)     │
         ┌──────────▼──────────┐  ┌──────────▼──────────┐
         │  SELECT / READ      │  │ INSERT/UPDATE/DROP  │
         │  Load-balance       │  │ Send to Master      │
         └──────────┬──────────┘  └──────────┬──────────┘
                    │                         │
      ┌─────────────┴──────────┐  ┌──────────▼─────────┐
      │                        │  │                    │
      ▼                        ▼  ▼                    │
   ┌──────────┐          ┌──────────────────┐         │
   │ Worker 1 │          │  Master Node     │         │
   │ (C++)    │          │  - DB Write      │         │
   │          │          │  - WAL fsynced   │         │
   │ Read-only│          │  - Replication   │         │
   │ MySQL    │          │  - Leadership    │         │
   └──────────┘          └────────┬─────────┘         │
      ▲                           │                   │
      │                           │ broadcast        │
      │                    ┌──────▼────────┐         │
      │                    │ Replication   │         │
      │         ┌──────────┤ Log / MQ      │◄────────┘
      │         │          └───────────────┘
      │         │
      ▼         ▼
   ┌──────────────┐
   │  Worker 2    │
   │  (Python)    │
   │              │
   │ Read-only    │
   │ MySQL        │
   └──────────────┘

═══════════════════════════════════════════════════
Master Node:       DB Write Access, Broadcast to Slaves
Worker Nodes:      Read-only DB, Listen to Replication Log
═══════════════════════════════════════════════════
```

### Data Flow

1. **Client Request** → API Gateway (port 8000)
2. **Gateway** validates auth, rate-limits, routes based on operation:
   - **Read (SELECT)** → Load-balance across Worker 1 & 2
   - **Write (INSERT/UPDATE/DELETE/CREATE)** → Master only
3. **Master** (port 8080):
   - Executes writes
   - Writes to WAL and fsyncs before applying locally (durability)
   - Broadcasts replicated mutations to Workers
4. **Workers** (ports 8081, 8082):
   - Apply replicated mutations from Master's log
   - Execute reads independently
   - Return results to Gateway
5. **Gateway** aggregates responses and returns to client

### Replication & Failover

- **Write-Ahead Log (WAL)** — Every mutation on Master is persisted to disk before applying (durability guarantee)
- **Master→Worker replication** — Asynchronous broadcast of WAL entries; Workers ACK receipt
- **Bully Election** — If Master fails, highest-ID Worker becomes Master and resumes writes
- **Crash Recovery** — Replay WAL on restart; Workers sync from Master's latest sequence number
- **Split-brain prevention** — Master token validates inter-node communication; external writes rejected

### Security

- **X-Master-Token** — HMAC-SHA256 token for inter-node communication (signed with shared `HMAC_SECRET`)
- **Client API Key** — Prevents unauthorized external queries to gateway
- **Replication blocked from outside** — Only Master can accept `X-Master-Token` and mutations
- **No external direct DB access** — All queries must route through API Gateway

---

## Components Documentation

### API Gateway (`api-gateway/`)

Routes client requests to Master (for writes) or Workers (for reads). Enforces rate-limiting and authentication.

**Files:**

- `cmd/gateway/main.go` — Server entry point
- `internal/router/` — Request routing logic
- `internal/auth/` — Token signing/verification
- `internal/ratelimit/` — Per-IP rate limiter
- `config/gateway.yaml` — Configuration

**Build:**

```bash
cd api-gateway && go build -o gateway ./cmd/gateway
```

---

### Master (`master/`)

Coordinates writes, manages replication, runs Bully election for failover.

**Files:**

- `cmd/master/main.go` — Server entry point
- `internal/db/` — SQL query execution
- `internal/wal/` — Write-Ahead Log
- `internal/replication/` — Master→Worker sync
- `internal/election/` — Bully algorithm
- `internal/cluster/` — Cluster state & discovery
- `config/master.yaml` — Configuration

**Build:**

```bash
cd master && go build -o master ./cmd/master
```

---

### Worker 1 (`worker-node1/`)

C++ read replica using Crow framework.

**Files:**

- `src/main.cpp` — Server entry point
- `src/query_handler.cpp` — SQL execution
- `src/replication.cpp` — Apply Master mutations
- `include/crow_all.h` — Crow framework (download via script)

**Build:**

```bash
cd worker-node1 && g++ -std=c++17 -O2 src/*.cpp -o worker
```

---

### Worker 2 (`worker-node2/`)

Python read replica using FastAPI.

**Files:**

- `main.py` — Server entry point
- `handlers/` — Request handlers
- `models/` — SQLAlchemy ORM models
- `requirements.txt` — Dependencies

**Build:**

```bash
pip install -r worker-node2/requirements.txt
python worker-node2/main.py
```

---

### GUI (`gui/`)

Streamlit web interface for queries, table browsing, and cluster monitoring.

**Files:**

- `app.py` — Main app

**Run:**

```bash
pip install streamlit
streamlit run gui/app.py
```

---

## Configuration

Each service reads from a YAML config file:

- **Master**: `master/config/master.yaml`
- **Gateway**: `api-gateway/config/gateway.yaml`
- **Workers**: Hardcoded in their source (modify and rebuild)

Key variables:

- `HMAC_SECRET` — Shared signing secret (set via `.env`)
- `NODE_ID` — Unique node identifier (Master=1, Worker1=2, Worker2=3)
- `CLUSTER_NODES` — List of all node addresses
- `LISTEN_PORT` — HTTP listen port

---

## Testing

### Automated tests

```bash
./scripts/test_cluster.sh
```

### Manual verification

1. Open GUI at `http://localhost:8501`
2. Create a database and table
3. Insert rows — verify they appear on both Workers
4. Kill Master container — verify Worker is elected
5. Query data — verify reads still work
6. Restart Master — verify it rejoins cluster

---

## Troubleshooting

| Issue | Solution |
|---|---|
| `Package not found` error on build | Ensure `go.mod` exists in module root; use `go mod tidy` |
| Workers can't connect to Master | Check `CLUSTER_NODES` in config; ensure containers share network |
| Replication lag | Check network latency; verify WAL is syncing (see logs) |
| GUI won't load | Ensure Streamlit container is running; visit `localhost:8501` |
| Master election stuck | Check node IDs are unique; verify all nodes can reach each other |

---

## License

MIT

---

## Contributing

Pull requests welcome. Please include tests and update the README for new features.
