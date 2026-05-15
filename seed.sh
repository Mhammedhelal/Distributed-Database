#!/usr/bin/env bash
# seed.sh — populates the cluster with demo data via the API Gateway.
# Usage: GATEWAY_URL=http://localhost:8000 ./scripts/seed.sh

set -euo pipefail

GW="${GATEWAY_URL:-http://localhost:8000}"
API_KEY="${GATEWAY_API_KEY:-}"

header_args=(-H "Content-Type: application/json")
[[ -n "$API_KEY" ]] && header_args+=(-H "Authorization: Bearer $API_KEY")

run_sql() {
    local sql="$1"
    echo "  SQL: $sql"
    curl -s -X POST "$GW/query" \
        "${header_args[@]}" \
        -d "{\"sql\": $(echo "$sql" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read().strip()))')}" \
        | python3 -m json.tool --no-ensure-ascii 2>/dev/null || true
    echo
}

echo "=== Creating database ==="
run_sql "CREATE DATABASE distdb"

echo "=== Creating tables ==="
run_sql "CREATE TABLE users (name VARCHAR(100), email VARCHAR(255), age INT)"
run_sql "CREATE TABLE products (title VARCHAR(200), description TEXT, price FLOAT, stock INT)"
run_sql "CREATE TABLE orders (user_id INT, product_id INT, quantity INT, status VARCHAR(50))"

echo "=== Inserting users ==="
run_sql "INSERT INTO users (name, email, age) VALUES ('Alice Smith', 'alice@example.com', 30)"
run_sql "INSERT INTO users (name, email, age) VALUES ('Bob Jones', 'bob@example.com', 25)"
run_sql "INSERT INTO users (name, email, age) VALUES ('Carol White', 'carol@example.com', 35)"
run_sql "INSERT INTO users (name, email, age) VALUES ('David Brown', 'david@example.com', 28)"

echo "=== Inserting products ==="
run_sql "INSERT INTO products (title, description, price, stock) VALUES ('Laptop Pro', 'High-performance laptop for developers', 1299.99, 50)"
run_sql "INSERT INTO products (title, description, price, stock) VALUES ('Wireless Mouse', 'Ergonomic wireless mouse', 29.99, 200)"
run_sql "INSERT INTO products (title, description, price, stock) VALUES ('Mechanical Keyboard', 'RGB mechanical keyboard', 89.99, 150)"
run_sql "INSERT INTO products (title, description, price, stock) VALUES ('4K Monitor', '27-inch 4K IPS display', 449.99, 75)"

echo "=== Inserting orders ==="
run_sql "INSERT INTO orders (user_id, product_id, quantity, status) VALUES (1, 1, 1, 'shipped')"
run_sql "INSERT INTO orders (user_id, product_id, quantity, status) VALUES (2, 3, 2, 'pending')"
run_sql "INSERT INTO orders (user_id, product_id, quantity, status) VALUES (3, 2, 1, 'delivered')"
run_sql "INSERT INTO orders (user_id, product_id, quantity, status) VALUES (1, 4, 1, 'processing')"

echo "=== Reading back from a worker (eventual consistency) ==="
run_sql "SELECT * FROM users"
run_sql "SELECT * FROM products"

echo ""
echo "✅ Seed complete. Open http://localhost:8501 to explore the data."