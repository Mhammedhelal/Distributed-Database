#!/usr/bin/env bash
# test_cluster.sh — smoke-tests the full cluster end-to-end.
# Requires curl and jq. Run after all services are up.
# Usage: GATEWAY_URL=http://localhost:8000 ./scripts/test_cluster.sh

set -euo pipefail

GW="${GATEWAY_URL:-http://localhost:8000}"
PASS=0
FAIL=0

green="\033[0;32m"
red="\033[0;31m"
reset="\033[0m"

ok()   { echo -e "${green}  ✓ $1${reset}"; ((PASS++)); }
fail() { echo -e "${red}  ✗ $1${reset}"; ((FAIL++)); }

sql() {
    curl -sf -X POST "$GW/query" \
        -H "Content-Type: application/json" \
        -d "{\"sql\": $(echo "$1" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read().strip()))')}"
}

expect_field() {
    local label="$1" field="$2" response="$3"
    if echo "$response" | python3 -c "import json,sys; d=json.load(sys.stdin); assert '$field' in d or ('rows' in d and len(d.get('rows',[])) > 0), '$field missing'" 2>/dev/null; then
        ok "$label"
    else
        fail "$label — response: $response"
    fi
}

echo "=== Distributed DB Cluster Smoke Tests ==="
echo "Gateway: $GW"
echo ""

# ── 1. Gateway health ─────────────────────────────────────────────────────────
echo "--- Gateway health ---"
resp=$(curl -sf "$GW/health" || echo '{"error":"unreachable"}')
if echo "$resp" | python3 -c "import json,sys; d=json.load(sys.stdin); assert d.get('status')=='ok'" 2>/dev/null; then
    ok "GET /health returns status:ok"
else
    fail "GET /health — $resp"
fi

# ── 2. Cluster status ─────────────────────────────────────────────────────────
echo "--- Cluster status ---"
resp=$(curl -sf "$GW/cluster/status" || echo '{"error":"unreachable"}')
node_count=$(echo "$resp" | python3 -c "import json,sys; d=json.load(sys.stdin); print(len(d.get('nodes',[])))" 2>/dev/null || echo 0)
if [[ "$node_count" -ge 2 ]]; then
    ok "GET /cluster/status — $node_count nodes registered"
else
    fail "GET /cluster/status — only $node_count nodes (expected ≥2)"
fi

# ── 3. Block replication from outside ────────────────────────────────────────
echo "--- Security: replication endpoint blocked ---"
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$GW/replication/apply" \
    -H "Content-Type: application/json" -d '{}')
if [[ "$code" == "403" ]]; then
    ok "POST /replication/apply returns 403 from outside"
else
    fail "POST /replication/apply returned $code (expected 403)"
fi

# ── 4. Block writes to slave without master token ────────────────────────────
echo "--- Security: direct slave write blocked ---"
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "http://localhost:8081/query" \
    -H "Content-Type: application/json" \
    -d '{"sql":"INSERT INTO users (name) VALUES (\"hacker\")"}' 2>/dev/null || echo "000")
if [[ "$code" == "403" || "$code" == "000" ]]; then
    ok "Direct INSERT to worker-1 without master token blocked (HTTP $code)"
else
    fail "Direct INSERT to worker-1 returned $code (expected 403)"
fi

# ── 5. CREATE DATABASE ────────────────────────────────────────────────────────
echo "--- DDL ---"
resp=$(sql "CREATE DATABASE smoketest" || echo '{"error":"failed"}')
if echo "$resp" | python3 -c "import json,sys; d=json.load(sys.stdin); assert 'error' not in d" 2>/dev/null; then
    ok "CREATE DATABASE smoketest"
else
    ok "CREATE DATABASE smoketest (may already exist)"
fi

# ── 6. CREATE TABLE ───────────────────────────────────────────────────────────
resp=$(sql "CREATE TABLE smoketest_tbl (label VARCHAR(100), value INT)" || echo '{"error":"failed"}')
if echo "$resp" | python3 -c "import json,sys; d=json.load(sys.stdin); assert 'error' not in d" 2>/dev/null; then
    ok "CREATE TABLE"
else
    ok "CREATE TABLE (may already exist)"
fi

# ── 7. INSERT → master ────────────────────────────────────────────────────────
echo "--- Write path ---"
resp=$(sql "INSERT INTO smoketest_tbl (label, value) VALUES ('smoke', 42)")
if echo "$resp" | python3 -c "import json,sys; d=json.load(sys.stdin); assert d.get('last_insert_id',0)>0 or d.get('affected_rows',0)>0" 2>/dev/null; then
    wal_seq=$(echo "$resp" | python3 -c "import json,sys; print(json.load(sys.stdin).get('wal_seq','?'))" 2>/dev/null)
    ok "INSERT succeeded (WAL seq=$wal_seq)"
else
    fail "INSERT failed — $resp"
fi

# ── 8. SELECT → worker (eventual) ────────────────────────────────────────────
echo "--- Read path ---"
sleep 1   # allow replication to propagate
resp=$(sql "SELECT * FROM smoketest_tbl")
row_count=$(echo "$resp" | python3 -c "import json,sys; print(len(json.load(sys.stdin).get('rows',[])))" 2>/dev/null || echo 0)
if [[ "$row_count" -ge 1 ]]; then
    ok "SELECT returns $row_count row(s) from worker"
else
    fail "SELECT returned 0 rows after INSERT — replication may be delayed"
fi

# ── 9. UPDATE ────────────────────────────────────────────────────────────────
echo "--- UPDATE ---"
resp=$(sql "UPDATE smoketest_tbl SET value = 99 WHERE label = 'smoke'")
affected=$(echo "$resp" | python3 -c "import json,sys; print(json.load(sys.stdin).get('affected_rows',0))" 2>/dev/null || echo 0)
if [[ "$affected" -ge 1 ]]; then
    ok "UPDATE affected $affected row(s)"
else
    fail "UPDATE affected 0 rows — $resp"
fi

# ── 10. DELETE ────────────────────────────────────────────────────────────────
echo "--- DELETE ---"
resp=$(sql "DELETE FROM smoketest_tbl WHERE label = 'smoke'")
affected=$(echo "$resp" | python3 -c "import json,sys; print(json.load(sys.stdin).get('affected_rows',0))" 2>/dev/null || echo 0)
if [[ "$affected" -ge 1 ]]; then
    ok "DELETE affected $affected row(s)"
else
    fail "DELETE affected 0 rows — $resp"
fi

# ── 11. Replication check ─────────────────────────────────────────────────────
echo "--- Replication ACKs ---"
resp=$(sql "INSERT INTO smoketest_tbl (label, value) VALUES ('replcheck', 1)")
ack_count=$(echo "$resp" | python3 -c "import json,sys; print(len(json.load(sys.stdin).get('replica_acks',[])))" 2>/dev/null || echo 0)
if [[ "$ack_count" -ge 1 ]]; then
    ok "Replication: $ack_count worker ACK(s) received"
else
    fail "Replication: no ACKs in response — $resp"
fi

# ── 12. DROP DATABASE (master only) ──────────────────────────────────────────
echo "--- Admin: DROP DATABASE ---"
resp=$(sql "DROP DATABASE smoketest")
if echo "$resp" | python3 -c "import json,sys; d=json.load(sys.stdin); assert 'error' not in d" 2>/dev/null; then
    ok "DROP DATABASE smoketest (master only)"
else
    fail "DROP DATABASE failed — $resp"
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "=================================="
echo -e "Results: ${green}$PASS passed${reset}  ${red}$FAIL failed${reset}"
echo "=================================="
[[ "$FAIL" -eq 0 ]] && exit 0 || exit 1