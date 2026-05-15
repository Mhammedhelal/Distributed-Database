// Package wal implements an append-only Write-Ahead Log.
// Every mutation is written to disk before being applied locally,
// enabling crash recovery and slave catch-up after downtime.
package wal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// OpType classifies the mutation recorded in a WAL entry.
type OpType string

const (
	OpInsert      OpType = "INSERT"
	OpUpdate      OpType = "UPDATE"
	OpDelete      OpType = "DELETE"
	OpCreateTable OpType = "CREATE_TABLE"
	OpDropTable   OpType = "DROP_TABLE"
	OpCreateDB    OpType = "CREATE_DB"
	OpDropDB      OpType = "DROP_DB"
)

// Entry is one mutation record in the WAL.
type Entry struct {
	Seq       uint64         `json:"seq"`
	Op        OpType         `json:"op"`
	Database  string         `json:"database"`
	Table     string         `json:"table,omitempty"`
	Data      map[string]any `json:"data,omitempty"`   // INSERT / UPDATE set values
	Where     string         `json:"where,omitempty"`  // UPDATE / DELETE filter
	WhereArgs []any          `json:"where_args,omitempty"`
	Cols      []ColDef       `json:"cols,omitempty"` // CREATE_TABLE columns
	Timestamp time.Time      `json:"ts"`
}

// ColDef mirrors db.ColDef to avoid a circular import.
type ColDef struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// WAL is an append-only, fsync-on-write log file.
type WAL struct {
	mu   sync.Mutex
	f    *os.File
	seq  uint64
	path string
}

// Open opens or creates the WAL at path, replaying existing entries to
// determine the current sequence number.
func Open(path string) (*WAL, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open wal %q: %w", path, err)
	}
	w := &WAL{f: f, path: path}
	// Determine highest seq already in the file.
	entries, err := w.ReadAll()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("replay wal: %w", err)
	}
	if len(entries) > 0 {
		w.seq = entries[len(entries)-1].Seq
	}
	return w, nil
}

// Append writes e to the WAL and fsyncs before returning.
func (w *WAL) Append(e *Entry) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.seq++
	e.Seq = w.seq
	e.Timestamp = time.Now().UTC()
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}
	b = append(b, '\n')
	if _, err := w.f.Write(b); err != nil {
		return fmt.Errorf("write wal: %w", err)
	}
	return w.f.Sync()
}

// ReadAll reads every entry from the beginning of the WAL file.
func (w *WAL) ReadAll() ([]Entry, error) {
	// Seek to start without locking — called only during Open.
	if _, err := w.f.Seek(0, 0); err != nil {
		return nil, err
	}
	var entries []Entry
	sc := bufio.NewScanner(w.f)
	sc.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("unmarshal wal line: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, sc.Err()
}

// Since returns all entries with Seq > afterSeq (for slave catch-up).
func (w *WAL) Since(afterSeq uint64) ([]Entry, error) {
	all, err := w.ReadAll()
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, e := range all {
		if e.Seq > afterSeq {
			out = append(out, e)
		}
	}
	return out, nil
}

// LastSeq returns the highest sequence number written so far.
func (w *WAL) LastSeq() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.seq
}

// Close flushes and closes the WAL file.
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.f.Close()
}