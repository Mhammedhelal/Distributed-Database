// Package query — executor.go
// Bridges parsed Statement objects to the db.Store and wal.WAL.
package query

import (
	"fmt"
	"strings"

	"github.com/distributed-db/master/internal/db"
	"github.com/distributed-db/master/internal/wal"
)

// Result is returned to the API handler after executing a statement.
type Result struct {
	Rows         []map[string]any `json:"rows,omitempty"`
	AffectedRows int64            `json:"affected_rows,omitempty"`
	LastInsertID int64            `json:"last_insert_id,omitempty"`
	Message      string           `json:"message,omitempty"`
	WALEntry     *wal.Entry       `json:"-"` // populated for write ops
}

// Executor runs statements against a Store and produces WAL entries.
type Executor struct {
	store    *db.Store
	isMaster bool // only masters may DROP DATABASE
}

// NewExecutor creates an Executor.
func NewExecutor(store *db.Store, isMaster bool) *Executor {
	return &Executor{store: store, isMaster: isMaster}
}

// Execute runs stmt and returns a Result.
func (e *Executor) Execute(stmt *Statement) (*Result, error) {
	switch stmt.Kind {
	case KindCreateDB:
		return e.createDB(stmt)
	case KindDropDB:
		return e.dropDB(stmt)
	case KindCreateTable:
		return e.createTable(stmt)
	case KindDropTable:
		return e.dropTable(stmt)
	case KindInsert:
		return e.insert(stmt)
	case KindSelect:
		return e.selectRows(stmt)
	case KindUpdate:
		return e.update(stmt)
	case KindDelete:
		return e.delete(stmt)
	default:
		return nil, fmt.Errorf("unknown statement kind: %s", stmt.Kind)
	}
}

func (e *Executor) createDB(stmt *Statement) (*Result, error) {
	if err := e.store.UseDB(stmt.Database); err != nil {
		return nil, err
	}
	return &Result{
		Message: fmt.Sprintf("Database %q created and selected", stmt.Database),
		WALEntry: &wal.Entry{
			Op:       wal.OpCreateDB,
			Database: stmt.Database,
		},
	}, nil
}

func (e *Executor) dropDB(stmt *Statement) (*Result, error) {
	if !e.isMaster {
		return nil, fmt.Errorf("DROP DATABASE is a master-only operation")
	}
	if err := e.store.DropDB(stmt.Database); err != nil {
		return nil, err
	}
	return &Result{
		Message: fmt.Sprintf("Database %q dropped", stmt.Database),
		WALEntry: &wal.Entry{
			Op:       wal.OpDropDB,
			Database: stmt.Database,
		},
	}, nil
}

func (e *Executor) createTable(stmt *Statement) (*Result, error) {
	dbCols := make([]db.ColDef, len(stmt.Cols))
	walCols := make([]wal.ColDef, len(stmt.Cols))
	for i, c := range stmt.Cols {
		dbCols[i] = db.ColDef{Name: c.Name, Type: c.Type}
		walCols[i] = wal.ColDef{Name: c.Name, Type: c.Type}
	}
	if err := e.store.CreateTable(stmt.Table, dbCols); err != nil {
		return nil, err
	}
	return &Result{
		Message: fmt.Sprintf("Table %q created", stmt.Table),
		WALEntry: &wal.Entry{
			Op:    wal.OpCreateTable,
			Table: stmt.Table,
			Cols:  walCols,
		},
	}, nil
}

func (e *Executor) dropTable(stmt *Statement) (*Result, error) {
	if err := e.store.DropTable(stmt.Table); err != nil {
		return nil, err
	}
	return &Result{
		Message: fmt.Sprintf("Table %q dropped", stmt.Table),
		WALEntry: &wal.Entry{
			Op:    wal.OpDropTable,
			Table: stmt.Table,
		},
	}, nil
}

func (e *Executor) insert(stmt *Statement) (*Result, error) {
	data := make(map[string]any, len(stmt.InsertCols))
	for i, col := range stmt.InsertCols {
		data[col] = stmt.InsertVals[i]
	}
	id, err := e.store.Insert(stmt.Table, data)
	if err != nil {
		return nil, err
	}
	return &Result{
		LastInsertID: id,
		AffectedRows: 1,
		WALEntry: &wal.Entry{
			Op:    wal.OpInsert,
			Table: stmt.Table,
			Data:  data,
		},
	}, nil
}

func (e *Executor) selectRows(stmt *Statement) (*Result, error) {
	where, args := whereSQL(stmt.Where)
	if stmt.Limit > 0 {
		where = appendLimit(where, stmt.Limit)
	}
	rows, err := e.store.Select(stmt.Table, where, args)
	if err != nil {
		return nil, err
	}
	return &Result{Rows: rows}, nil
}

func (e *Executor) update(stmt *Statement) (*Result, error) {
	where, args := whereSQL(stmt.Where)
	n, err := e.store.Update(stmt.Table, stmt.SetCols, where, args)
	if err != nil {
		return nil, err
	}
	return &Result{
		AffectedRows: n,
		WALEntry: &wal.Entry{
			Op:        wal.OpUpdate,
			Table:     stmt.Table,
			Data:      stmt.SetCols,
			Where:     where,
			WhereArgs: args,
		},
	}, nil
}

func (e *Executor) delete(stmt *Statement) (*Result, error) {
	where, args := whereSQL(stmt.Where)
	n, err := e.store.Delete(stmt.Table, where, args)
	if err != nil {
		return nil, err
	}
	return &Result{
		AffectedRows: n,
		WALEntry: &wal.Entry{
			Op:        wal.OpDelete,
			Table:     stmt.Table,
			Where:     where,
			WhereArgs: args,
		},
	}, nil
}

// whereSQL converts a WhereClause to a SQL fragment + args slice.
func whereSQL(wc *WhereClause) (string, []any) {
	if wc == nil {
		return "", nil
	}
	return fmt.Sprintf("`%s` %s ?", wc.Col, wc.Op), []any{wc.Val}
}

// appendLimit adds a LIMIT clause to an existing WHERE fragment.
func appendLimit(where string, limit int) string {
	suffix := fmt.Sprintf(" LIMIT %d", limit)
	if strings.TrimSpace(where) == "" {
		return suffix
	}
	return where + suffix
}

// DBName returns the active database name from the underlying store.
func (e *Executor) DBName() string { return e.store.DBName() }