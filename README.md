Here is your updated README. The full script for installing system dependencies, configuring MySQL, and setting up the individual language-specific modules has been cleanly integrated right under the **Prerequisites** section so users can set up their complete environment in one go.

---

# Distributed Database

A fully distributed, fault-tolerant database system with master-worker replication, automatic failover via Bully election, and a web-based query interface.

**Components:**

* **API Gateway** (Go) — Routes requests, enforces security, rate-limits
* **Master** (Go) — Writes, replication coordinator, leader election
* **Worker 1** (C++) — Read replica, query execution
* **Worker 2** (Python) — Read replica, query execution
* **GUI** (Streamlit) — Web interface for queries, tables, cluster health

---

## Quick Start — Get Running in 5 Minutes

### Prerequisites & Environment Setup

Run the following commands to install the required system packages, configure MySQL, and set up dependencies for all components:

```bash
# ── 1. System packages ─────────────────────────────────────────────────────────
sudo apt update && sudo apt install -y \
    golang-go \
    mysql-server \
    python3-pip \
    python3-venv \
    build-essential \
    cmake \
    libmysqlclient-dev \
    libssl-dev \
    libboost-dev \
    nlohmann-json3-dev \
    curl \
    git

# ── 2. Start MySQL ─────────────────────────────────────────────────────────────
sudo systemctl start mysql
sudo systemctl enable mysql

# ── 3. Set MySQL root password ─────────────────────────────────────────────────
sudo mysql << 'SQL'
ALTER USER 'root'@'localhost' IDENTIFIED WITH mysql_native_password BY 'rootpass';
FLUSH PRIVILEGES;
CREATE DATABASE IF NOT EXISTS distdb;
USE distdb;
SET GLOBAL max_allowed_packet = 67108864;
SET GLOBAL innodb_flush_log_at_trx_commit = 2;
FLUSH PRIVILEGES;
SQL

# ── 4. Python deps — Worker 2 ──────────────────────────────────────────────────
cd ~/dev/Distributed-Database/worker-node2
python3 -m venv venv
source venv/bin/activate
pip install -r requirements.txt
deactivate

# ── 5. Python deps — GUI ───────────────────────────────────────────────────────
cd ~/dev/Distributed-Database/gui
python3 -m venv venv
source venv/bin/activate
pip install -r requirements.txt
deactivate

# ── 6. Go deps — Master ────────────────────────────────────────────────────────
cd ~/dev/Distributed-Database/master
go mod download

# ── 7. Go deps — API Gateway ───────────────────────────────────────────────────
cd ~/dev/Distributed-Database/api-gateway
go mod download

# ── 8. C++ Worker 1 — download Crow and build ──────────────────────────────────
cd ~/dev/Distributed-Database/worker-node1
mkdir -p include
curl -L https://github.com/CrowCpp/Crow/releases/download/v1.1.0/crow_all.h \
     -o include/crow_all.h
cmake -B build -DCMAKE_BUILD_TYPE=Release
cmake --build build -j$(nproc)

echo ""
echo "✅ All requirements installed. Run each service in a separate terminal."

```

Verify your MySQL connection before proceeding:

```bash
mysql -u root -prootpass -e "SELECT 1;"

```

---

### 1 — Project Layout

Ensure your folder hierarchy matches this structure:

```
distributed-db/
├── api-gateway/
├── master/
├── worker-node1/
├── worker-node2/
├── gui/
├── scripts/
└── README.md

```

---

### 2 — Configure the secret

Create a `.env` file in the root directory (next to `README.md`):

```bash
HMAC_SECRET=my-super-secret-32-char-minimum!!

```

> ⚠️ **Important:** This shared secret is used by every node to sign and verify `X-Master-Token`. Never commit it to version control.

---

### 3 — Start the cluster (open 5 terminals)

**Terminal 1 — Master**

```bash
cd master
go run ./cmd/server

```

Wait for: `"msg":"master listening"`

**Terminal 2 — Worker 1 (C++)**

```bash
cd worker-node1
./build/worker_node1

```

Wait for: `worker-node1 listening on :8081`

**Terminal 3 — Worker 2 (Python)**

```bash
cd worker-node2
source venv/bin/activate
uvicorn app.main:app --host 0.0.0.0 --port 8082

```

Wait for: `Application startup complete`

**Terminal 4 — API Gateway**

```bash
cd api-gateway
HMAC_SECRET=my-super-secret-32-char-minimum!! go run ./cmd/gateway

```

Wait for: `"msg":"gateway listening"`

**Terminal 5 — GUI**

```bash
cd gui
source venv/bin/activate
GATEWAY_URL=http://localhost:8000 streamlit run app.py

```

The GUI will open automatically at **[http://localhost:8501](https://www.google.com/search?q=http://localhost:8501)**.

---

### 4 — Seed Sample Data

In a new terminal, execute the seed script:

```bash
GATEWAY_URL=http://localhost:8000 ./scripts/seed.sh

```

This initializes the `distdb` database with `users`, `products`, and `orders` tables containing mock data. Navigate to the **Table Browser** in the Streamlit UI and type `distdb` to explore.

---

## Usage Examples

### Run the smoke tests

```bash
GATEWAY_URL=http://localhost:8000 ./scripts/test_cluster.sh

```

Runs 12 automated checks covering: health states, perimeter security (blocking replication payloads from external sources, blocking non-token writes on slaves), full CRUD lifecycle validation, replication ACKs, and `DROP DATABASE` handling.

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

See the Bully election protocol handle worker transitions in real-time:

```bash
./scripts/failover_demo.sh

```

This halts the master node container, pauses 15 seconds for a high-priority worker node to claim leadership, verifies that data reads remain accessible, restarts the primary master, and confirms its safe cluster re-entry.

---

## Service ports

| Service | Port | URL |
| --- | --- | --- |
| API Gateway | 8000 | `http://localhost:8000` |
| Master (Go) | 8080 | `http://localhost:8080` |
| Worker 1 (C++) | 8081 | `http://localhost:8081` |
| Worker 2 (Python) | 8082 | `http://localhost:8082` |
| Streamlit GUI | 8501 | `http://localhost:8501` |

---

## Stopping services

To stop any active cluster engine component, press `Ctrl+C` inside its dedicated execution terminal window.

To completely drop and flush transient system data profiles inside MySQL, run:

```bash
sudo mysql -u root -prootpass -e "DROP DATABASE distdb; CREATE DATABASE distdb;"

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
* **Read (SELECT)** → Load-balance across Worker 1 & 2
* **Write (INSERT/UPDATE/DELETE/CREATE)** → Master only


3. **Master** (port 8080):
* Executes writes
* Writes to WAL and fsyncs before applying locally (durability)
* Broadcasts replicated mutations to Workers


4. **Workers** (ports 8081, 8082):
* Apply replicated mutations from Master's log
* Execute reads independently
* Return results to Gateway


5. **Gateway** aggregates responses and returns to client

### Replication & Failover

* **Write-Ahead Log (WAL)** — Every mutation on Master is persisted to disk before applying (durability guarantee)
* **Master→Worker replication** — Asynchronous broadcast of WAL entries; Workers ACK receipt
* **Bully Election** — If Master fails, highest-ID Worker becomes Master and resumes writes
* **Crash Recovery** — Replay WAL on restart; Workers sync from Master's latest sequence number
* **Split-brain prevention** — Master token validates inter-node communication; external writes rejected

### Security

* **X-Master-Token** — HMAC-SHA256 token for inter-node communication (signed with shared `HMAC_SECRET`)
* **Client API Key** — Prevents unauthorized external queries to gateway
* **Replication blocked from outside** — Only Master can accept `X-Master-Token` and mutations
* **No external direct DB access** — All queries must route through API Gateway

---

## Components Documentation

### API Gateway (`api-gateway/`)

Routes client requests to Master (for writes) or Workers (for reads). Enforces rate-limiting and authentication.
**Files:**

* `cmd/gateway/main.go` — Server entry point
* `internal/router/` — Request routing logic
* `internal/auth/` — Token signing/verification
* `internal/ratelimit/` — Per-IP rate limiter
* `config/gateway.yaml` — Configuration

**Build:**

```bash
cd api-gateway && go build -o gateway ./cmd/gateway

```

---

### Master (`master/`)

Coordinates writes, manages replication, runs Bully election for failover.
**Files:**

* `cmd/master/main.go` — Server entry point
* `internal/db/` — SQL query execution
* `internal/wal/` — Write-Ahead Log
* `internal/replication/` — Master→Worker sync
* `internal/election/` — Bully algorithm
* `internal/cluster/` — Cluster state & discovery
* `config/master.yaml` — Configuration

**Build:**

```bash
cd master && go build -o master ./cmd/master

```

---

### Worker 1 (`worker-node1/`)

C++ read replica using Crow framework.
**Files:**

* `src/main.cpp` — Server entry point
* `src/query_handler.cpp` — SQL execution
* `src/replication.cpp` — Apply Master mutations
* `include/crow_all.h` — Crow framework (downloaded during setup)

**Build:**

```bash
cd worker-node1 && g++ -std=c++17 -O2 src/*.cpp -o worker

```

---

### Worker 2 (`worker-node2/`)

Python read replica using FastAPI.
**Files:**

* `main.py` — Server entry point
* `handlers/` — Request handlers
* `models/` — SQLAlchemy ORM models
* `requirements.txt` — Dependencies

**Run:**

```bash
source worker-node2/venv/bin/activate
python worker-node2/main.py

```

---

### GUI (`gui/`)

Streamlit web interface for queries, table browsing, and cluster monitoring.
**Files:**

* `app.py` — Main app

**Run:**

```bash
source gui/venv/bin/activate
streamlit run gui/app.py

```

---

## Configuration

Each service reads from a YAML config file:

* **Master**: `master/config/master.yaml`
* **Gateway**: `api-gateway/config/gateway.yaml`
* **Workers**: Hardcoded in their source (modify and rebuild)

Key variables:

* `HMAC_SECRET` — Shared signing secret (set via `.env`)
* `NODE_ID` — Unique node identifier (Master=1, Worker1=2, Worker2=3)
* `CLUSTER_NODES` — List of all node addresses
* `LISTEN_PORT` — HTTP listen port

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
4. Kill Master process — verify Worker is elected
5. Query data — verify reads still work
6. Restart Master — verify it rejoins cluster

---

## Troubleshooting

| Issue | Solution |
| --- | --- |
| `Package not found` error on build | Ensure `go.mod` exists in module root; use `go mod tidy` |
| Workers can't connect to Master | Check `CLUSTER_NODES` in config; ensure containers/hosts share network connectivity |
| Replication lag | Check network latency; verify WAL is syncing (see logs) |
| GUI won't load | Ensure python virtual environment is activated and package dependencies match |
| Master election stuck | Check node IDs are unique; verify all nodes can reach each other via TCP/UDP |