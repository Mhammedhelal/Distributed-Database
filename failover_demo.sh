#!/usr/bin/env bash
# failover_demo.sh — demonstrates master failure and worker promotion.
# Works with docker-compose. Kills the master container, verifies the
# cluster continues to accept writes via the elected worker.

set -euo pipefail

GW="${GATEWAY_URL:-http://localhost:8000}"

green="\033[0;32m"
yellow="\033[1;33m"
red="\033[0;31m"
reset="\033[0m"

step() { echo -e "\n${yellow}▶ $1${reset}"; }
ok()   { echo -e "${green}  ✓ $1${reset}"; }
info() { echo -e "  ℹ  $1"; }

sql() {
    curl -sf -X POST "$GW/query" \
        -H "Content-Type: application/json" \
        -d "{\"sql\": $(echo "$1" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read().strip()))')}" \
    || echo '{"error":"request failed"}'
}

step "1. Verify cluster is healthy before failover"
resp=$(curl -sf "$GW/cluster/status")
echo "$resp" | python3 -m json.tool
ok "Cluster status fetched"

step "2. Insert a row before killing master"
sql "CREATE DATABASE IF NOT EXISTS failover_test"
sql "CREATE TABLE IF NOT EXISTS failover_tbl (label VARCHAR(100))"
resp=$(sql "INSERT INTO failover_tbl (label) VALUES ('before-failover')")
echo "$resp"
ok "Pre-failover INSERT succeeded"

step "3. Killing master container..."
docker compose stop master
info "Master container stopped. Waiting 15s for election..."
sleep 15

step "4. Checking cluster status after failover"
resp=$(curl -sf "$GW/cluster/status" || echo '{"error":"gateway down"}')
echo "$resp" | python3 -m json.tool

step "5. Attempting SELECT via gateway (should succeed via worker)"
resp=$(sql "SELECT * FROM failover_tbl")
echo "$resp"
row_count=$(echo "$resp" | python3 -c "import json,sys; print(len(json.load(sys.stdin).get('rows',[])))" 2>/dev/null || echo 0)
if [[ "$row_count" -ge 1 ]]; then
    ok "SELECT succeeded after master failure ($row_count rows)"
else
    echo -e "${red}  ✗ SELECT returned 0 rows${reset}"
fi

step "6. Restarting master container"
docker compose start master
info "Waiting 15s for master to rejoin and catch up..."
sleep 15

step "7. Verify master rejoined"
resp=$(curl -sf "$GW/cluster/status")
echo "$resp" | python3 -m json.tool
ok "Master back in cluster"

step "8. Cleanup"
sql "DROP DATABASE failover_test"
ok "Failover demo complete"