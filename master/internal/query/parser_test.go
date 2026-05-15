package query_test

import (
	"testing"

	"master/internal/query"
)

// ── CREATE DATABASE ───────────────────────────────────────────────────────────

func TestParseCreateDatabase(t *testing.T) {
	stmt, err := query.Parse("CREATE DATABASE mydb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stmt.Kind != query.KindCreateDB {
		t.Errorf("kind: want %s, got %s", query.KindCreateDB, stmt.Kind)
	}
	if stmt.Database != "mydb" {
		t.Errorf("database: want mydb, got %s", stmt.Database)
	}
}

// ── DROP DATABASE ─────────────────────────────────────────────────────────────

func TestParseDropDatabase(t *testing.T) {
	stmt, err := query.Parse("DROP DATABASE mydb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stmt.Kind != query.KindDropDB {
		t.Errorf("kind: want %s, got %s", query.KindDropDB, stmt.Kind)
	}
	if stmt.Database != "mydb" {
		t.Errorf("database: want mydb, got %s", stmt.Database)
	}
}

// ── CREATE TABLE ──────────────────────────────────────────────────────────────

func TestParseCreateTable(t *testing.T) {
	stmt, err := query.Parse("CREATE TABLE users (name VARCHAR(100), age INT)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stmt.Kind != query.KindCreateTable {
		t.Errorf("kind: want %s, got %s", query.KindCreateTable, stmt.Kind)
	}
	if stmt.Table != "users" {
		t.Errorf("table: want users, got %s", stmt.Table)
	}
	if len(stmt.Cols) != 2 {
		t.Fatalf("cols: want 2, got %d", len(stmt.Cols))
	}
	if stmt.Cols[0].Name != "name" || stmt.Cols[0].Type != "VARCHAR(100)" {
		t.Errorf("col[0]: want name VARCHAR(100), got %s %s", stmt.Cols[0].Name, stmt.Cols[0].Type)
	}
	if stmt.Cols[1].Name != "age" || stmt.Cols[1].Type != "INT" {
		t.Errorf("col[1]: want age INT, got %s %s", stmt.Cols[1].Name, stmt.Cols[1].Type)
	}
}

func TestParseCreateTableMultiWordType(t *testing.T) {
	stmt, err := query.Parse("CREATE TABLE events (ts DATETIME NOT NULL, payload TEXT)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stmt.Cols[0].Type != "DATETIME NOT NULL" {
		t.Errorf("multi-word type: want 'DATETIME NOT NULL', got %q", stmt.Cols[0].Type)
	}
}

// ── DROP TABLE ────────────────────────────────────────────────────────────────

func TestParseDropTable(t *testing.T) {
	stmt, err := query.Parse("DROP TABLE orders")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stmt.Kind != query.KindDropTable || stmt.Table != "orders" {
		t.Errorf("got kind=%s table=%s", stmt.Kind, stmt.Table)
	}
}

// ── INSERT ────────────────────────────────────────────────────────────────────

func TestParseInsert(t *testing.T) {
	stmt, err := query.Parse("INSERT INTO users (name, age) VALUES ('Alice', 30)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stmt.Kind != query.KindInsert {
		t.Errorf("kind: want INSERT, got %s", stmt.Kind)
	}
	if stmt.Table != "users" {
		t.Errorf("table: want users, got %s", stmt.Table)
	}
	if len(stmt.InsertCols) != 2 || stmt.InsertCols[0] != "name" || stmt.InsertCols[1] != "age" {
		t.Errorf("insert cols: %v", stmt.InsertCols)
	}
	if stmt.InsertVals[0] != "Alice" {
		t.Errorf("val[0]: want Alice, got %v", stmt.InsertVals[0])
	}
	if stmt.InsertVals[1] != float64(30) {
		t.Errorf("val[1]: want 30.0, got %v", stmt.InsertVals[1])
	}
}

func TestParseInsertMismatchedColsVals(t *testing.T) {
	_, err := query.Parse("INSERT INTO t (a, b) VALUES ('x')")
	if err == nil {
		t.Error("expected error for mismatched cols/vals, got nil")
	}
}

// ── SELECT ────────────────────────────────────────────────────────────────────

func TestParseSelectStar(t *testing.T) {
	stmt, err := query.Parse("SELECT * FROM products")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stmt.Kind != query.KindSelect || stmt.Table != "products" {
		t.Errorf("got kind=%s table=%s", stmt.Kind, stmt.Table)
	}
	if stmt.Where != nil {
		t.Error("expected no WHERE clause")
	}
}

func TestParseSelectWithWhere(t *testing.T) {
	stmt, err := query.Parse("SELECT * FROM users WHERE age = 30")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stmt.Where == nil {
		t.Fatal("expected WHERE clause, got nil")
	}
	if stmt.Where.Col != "age" || stmt.Where.Op != "=" || stmt.Where.Val != float64(30) {
		t.Errorf("where: got col=%s op=%s val=%v", stmt.Where.Col, stmt.Where.Op, stmt.Where.Val)
	}
}

func TestParseSelectWithLimit(t *testing.T) {
	stmt, err := query.Parse("SELECT * FROM users LIMIT 10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stmt.Limit != 10 {
		t.Errorf("limit: want 10, got %d", stmt.Limit)
	}
}

func TestParseSelectWhereAndLimit(t *testing.T) {
	stmt, err := query.Parse("SELECT * FROM users WHERE name = 'Bob' LIMIT 5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stmt.Where == nil || stmt.Where.Val != "Bob" {
		t.Errorf("where val: want Bob, got %v", stmt.Where)
	}
	if stmt.Limit != 5 {
		t.Errorf("limit: want 5, got %d", stmt.Limit)
	}
}

func TestParseSelectComparisonOperators(t *testing.T) {
	cases := []struct {
		sql string
		op  string
	}{
		{"SELECT * FROM t WHERE age != 10", "!="},
		{"SELECT * FROM t WHERE age < 10", "<"},
		{"SELECT * FROM t WHERE age <= 10", "<="},
		{"SELECT * FROM t WHERE age > 10", ">"},
		{"SELECT * FROM t WHERE age >= 10", ">="},
	}
	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			stmt, err := query.Parse(tc.sql)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if stmt.Where.Op != tc.op {
				t.Errorf("op: want %s, got %s", tc.op, stmt.Where.Op)
			}
		})
	}
}

// ── UPDATE ────────────────────────────────────────────────────────────────────

func TestParseUpdate(t *testing.T) {
	stmt, err := query.Parse("UPDATE users SET name = 'Bob', age = 26 WHERE id = 1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stmt.Kind != query.KindUpdate || stmt.Table != "users" {
		t.Errorf("kind=%s table=%s", stmt.Kind, stmt.Table)
	}
	if len(stmt.SetCols) != 2 {
		t.Errorf("set cols: want 2, got %d", len(stmt.SetCols))
	}
	if stmt.SetCols["name"] != "Bob" {
		t.Errorf("name: want Bob, got %v", stmt.SetCols["name"])
	}
	if stmt.Where == nil || stmt.Where.Col != "id" {
		t.Errorf("where: %v", stmt.Where)
	}
}

func TestParseUpdateNoWhere(t *testing.T) {
	stmt, err := query.Parse("UPDATE products SET stock = 0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stmt.Where != nil {
		t.Error("expected no WHERE clause")
	}
}

// ── DELETE ────────────────────────────────────────────────────────────────────

func TestParseDelete(t *testing.T) {
	stmt, err := query.Parse("DELETE FROM users WHERE id = 5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stmt.Kind != query.KindDelete || stmt.Table != "users" {
		t.Errorf("kind=%s table=%s", stmt.Kind, stmt.Table)
	}
	if stmt.Where == nil || stmt.Where.Col != "id" || stmt.Where.Val != float64(5) {
		t.Errorf("where: %v", stmt.Where)
	}
}

func TestParseDeleteNoWhere(t *testing.T) {
	stmt, err := query.Parse("DELETE FROM sessions")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stmt.Where != nil {
		t.Error("expected no WHERE clause")
	}
}

// ── Error cases ───────────────────────────────────────────────────────────────

func TestParseUnknownKeyword(t *testing.T) {
	_, err := query.Parse("TRUNCATE TABLE users")
	if err == nil {
		t.Error("expected error for unsupported statement, got nil")
	}
}

func TestParseEmpty(t *testing.T) {
	_, err := query.Parse("")
	if err == nil {
		t.Error("expected error for empty input, got nil")
	}
}

func TestParseCaseInsensitive(t *testing.T) {
	stmt, err := query.Parse("select * from Users where Id = 1")
	if err != nil {
		t.Fatalf("case-insensitive parse failed: %v", err)
	}
	if stmt.Kind != query.KindSelect {
		t.Errorf("want SELECT, got %s", stmt.Kind)
	}
}

func TestParseStringWithSpaces(t *testing.T) {
	stmt, err := query.Parse("INSERT INTO t (label) VALUES ('hello world')")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stmt.InsertVals[0] != "hello world" {
		t.Errorf("string value: want 'hello world', got %v", stmt.InsertVals[0])
	}
}

func TestParseNullValue(t *testing.T) {
	stmt, err := query.Parse("INSERT INTO t (label) VALUES (NULL)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stmt.InsertVals[0] != nil {
		t.Errorf("null value: want nil, got %v", stmt.InsertVals[0])
	}
}

func TestParseFloatValue(t *testing.T) {
	stmt, err := query.Parse("INSERT INTO products (price) VALUES (9.99)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stmt.InsertVals[0] != 9.99 {
		t.Errorf("float value: want 9.99, got %v", stmt.InsertVals[0])
	}
}