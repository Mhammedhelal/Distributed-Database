// Package db wraps a MySQL connection pool and provides schema + CRUD operations.
package db

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

// Store is the master-node's local MySQL database handle.
type Store struct {
	db     *sql.DB
	dbName string
}

// New opens a connection pool to MySQL and pings it.
func New(dsn string) (*Store, error) {
	d, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	if err := d.Ping(); err != nil {
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	d.SetMaxOpenConns(25)
	d.SetMaxIdleConns(10)
	return &Store{db: d}, nil
}

// UseDB selects or creates the active database.
func (s *Store) UseDB(name string) error {
	_, err := s.db.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", name))
	if err != nil {
		return fmt.Errorf("create db %q: %w", name, err)
	}
	_, err = s.db.Exec(fmt.Sprintf("USE `%s`", name))
	if err != nil {
		return fmt.Errorf("use db %q: %w", name, err)
	}
	s.dbName = name
	return nil
}

// DropDB drops the named database — master-only operation.
func (s *Store) DropDB(name string) error {
	_, err := s.db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", name))
	return err
}

// ColDef describes a column during CREATE TABLE.
type ColDef struct {
	Name string
	Type string // e.g. "VARCHAR(255)", "INT", "TEXT"
}

// CreateTable creates a table dynamically from ColDef slice.
func (s *Store) CreateTable(table string, cols []ColDef) error {
	if len(cols) == 0 {
		return fmt.Errorf("at least one column required")
	}
	defs := make([]string, 0, len(cols)+1)
	defs = append(defs, "`id` INT AUTO_INCREMENT PRIMARY KEY")
	for _, c := range cols {
		defs = append(defs, fmt.Sprintf("`%s` %s", c.Name, c.Type))
	}
	q := fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s` (%s)", table, strings.Join(defs, ", "))
	_, err := s.db.Exec(q)
	return err
}

// DropTable drops a table.
func (s *Store) DropTable(table string) error {
	_, err := s.db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", table))
	return err
}

// Insert inserts a row and returns the new id.
func (s *Store) Insert(table string, data map[string]any) (int64, error) {
	cols := make([]string, 0, len(data))
	placeholders := make([]string, 0, len(data))
	vals := make([]any, 0, len(data))
	for k, v := range data {
		cols = append(cols, fmt.Sprintf("`%s`", k))
		placeholders = append(placeholders, "?")
		vals = append(vals, v)
	}
	q := fmt.Sprintf("INSERT INTO `%s` (%s) VALUES (%s)",
		table, strings.Join(cols, ","), strings.Join(placeholders, ","))
	res, err := s.db.Exec(q, vals...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Select returns rows matching where clause (raw SQL fragment, safe for internal use).
func (s *Store) Select(table, where string, args []any) ([]map[string]any, error) {
	q := fmt.Sprintf("SELECT * FROM `%s`", table)
	if where != "" {
		q += " WHERE " + where
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

// Update updates rows matching where and returns affected count.
func (s *Store) Update(table string, set map[string]any, where string, args []any) (int64, error) {
	setParts := make([]string, 0, len(set))
	setVals := make([]any, 0, len(set))
	for k, v := range set {
		setParts = append(setParts, fmt.Sprintf("`%s`=?", k))
		setVals = append(setVals, v)
	}
	q := fmt.Sprintf("UPDATE `%s` SET %s", table, strings.Join(setParts, ","))
	if where != "" {
		q += " WHERE " + where
	}
	allArgs := append(setVals, args...)
	res, err := s.db.Exec(q, allArgs...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Delete deletes rows matching where and returns affected count.
func (s *Store) Delete(table, where string, args []any) (int64, error) {
	q := fmt.Sprintf("DELETE FROM `%s`", table)
	if where != "" {
		q += " WHERE " + where
	}
	res, err := s.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ListTables lists all tables in the current database.
func (s *Store) ListTables() ([]string, error) {
	rows, err := s.db.Query("SHOW TABLES")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	return tables, rows.Err()
}

// Close closes the underlying pool.
func (s *Store) Close() error { return s.db.Close() }

// DBName returns the active database name.
func (s *Store) DBName() string { return s.dbName }

// Raw executes a raw SQL statement (used by replication replay).
func (s *Store) Raw(query string, args ...any) error {
	_, err := s.db.Exec(query, args...)
	return err
}

// scanRows converts *sql.Rows into a slice of string-keyed maps.
func scanRows(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var result []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			b, ok := vals[i].([]byte)
			if ok {
				row[c] = string(b)
			} else {
				row[c] = vals[i]
			}
		}
		result = append(result, row)
	}
	return result, rows.Err()
}