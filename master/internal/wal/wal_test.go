package wal_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"master/internal/wal"
)

func tempWAL(t *testing.T) (*wal.WAL, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.wal")
	w, err := wal.Open(path)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	t.Cleanup(func() { w.Close() })
	return w, path
}

func TestAppendAndReadAll(t *testing.T) {
	w, _ := tempWAL(t)

	entries := []*wal.Entry{
		{Op: wal.OpCreateDB, Database: "testdb"},
		{Op: wal.OpCreateTable, Database: "testdb", Table: "users"},
		{Op: wal.OpInsert, Database: "testdb", Table: "users", Data: map[string]any{"name": "Alice"}},
	}
	for _, e := range entries {
		if err := w.Append(e); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	all, err := w.ReadAll()
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("want 3 entries, got %d", len(all))
	}
}

func TestSeqMonotonicallyIncreases(t *testing.T) {
	w, _ := tempWAL(t)

	for i := 0; i < 5; i++ {
		if err := w.Append(&wal.Entry{Op: wal.OpInsert, Table: "t"}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	all, _ := w.ReadAll()
	for i := 1; i < len(all); i++ {
		if all[i].Seq <= all[i-1].Seq {
			t.Errorf("seq not monotonic: entry[%d].Seq=%d <= entry[%d].Seq=%d",
				i, all[i].Seq, i-1, all[i-1].Seq)
		}
	}
}

func TestLastSeq(t *testing.T) {
	w, _ := tempWAL(t)

	if w.LastSeq() != 0 {
		t.Errorf("want initial seq 0, got %d", w.LastSeq())
	}
	w.Append(&wal.Entry{Op: wal.OpInsert, Table: "t"})
	w.Append(&wal.Entry{Op: wal.OpInsert, Table: "t"})
	if w.LastSeq() != 2 {
		t.Errorf("want seq 2, got %d", w.LastSeq())
	}
}

func TestSince(t *testing.T) {
	w, _ := tempWAL(t)

	for i := 0; i < 5; i++ {
		w.Append(&wal.Entry{Op: wal.OpInsert, Table: "t"})
	}

	since, err := w.Since(3)
	if err != nil {
		t.Fatalf("since: %v", err)
	}
	if len(since) != 2 {
		t.Errorf("Since(3): want 2 entries, got %d", len(since))
	}
	for _, e := range since {
		if e.Seq <= 3 {
			t.Errorf("Since(3) returned entry with seq %d", e.Seq)
		}
	}
}

func TestSinceZeroReturnsAll(t *testing.T) {
	w, _ := tempWAL(t)
	w.Append(&wal.Entry{Op: wal.OpInsert, Table: "t"})
	w.Append(&wal.Entry{Op: wal.OpInsert, Table: "t"})

	since, err := w.Since(0)
	if err != nil {
		t.Fatalf("since(0): %v", err)
	}
	if len(since) != 2 {
		t.Errorf("Since(0): want 2, got %d", len(since))
	}
}

func TestTimestampIsSet(t *testing.T) {
	w, _ := tempWAL(t)
	before := time.Now().UTC()
	e := &wal.Entry{Op: wal.OpInsert, Table: "t"}
	w.Append(e)
	after := time.Now().UTC()

	all, _ := w.ReadAll()
	if len(all) == 0 {
		t.Fatal("no entries")
	}
	ts := all[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp %v outside [%v, %v]", ts, before, after)
	}
}

func TestCrashRecovery(t *testing.T) {
	// Write entries to a WAL, close it, reopen, and verify seq continues.
	dir := t.TempDir()
	path := filepath.Join(dir, "recovery.wal")

	w1, err := wal.Open(path)
	if err != nil {
		t.Fatalf("open w1: %v", err)
	}
	w1.Append(&wal.Entry{Op: wal.OpInsert, Table: "t"})
	w1.Append(&wal.Entry{Op: wal.OpUpdate, Table: "t"})
	lastSeq := w1.LastSeq()
	w1.Close()

	// Reopen — simulate crash recovery.
	w2, err := wal.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer w2.Close()

	if w2.LastSeq() != lastSeq {
		t.Errorf("after reopen: want lastSeq=%d, got %d", lastSeq, w2.LastSeq())
	}

	// Appending after reopen should increment from lastSeq.
	w2.Append(&wal.Entry{Op: wal.OpDelete, Table: "t"})
	if w2.LastSeq() != lastSeq+1 {
		t.Errorf("after append: want seq=%d, got %d", lastSeq+1, w2.LastSeq())
	}
}

func TestAllOpTypesRoundTrip(t *testing.T) {
	w, _ := tempWAL(t)

	ops := []wal.OpType{
		wal.OpInsert, wal.OpUpdate, wal.OpDelete,
		wal.OpCreateTable, wal.OpDropTable,
		wal.OpCreateDB, wal.OpDropDB,
	}
	for _, op := range ops {
		w.Append(&wal.Entry{Op: op, Database: "db", Table: "t"})
	}

	all, err := w.ReadAll()
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	if len(all) != len(ops) {
		t.Fatalf("want %d entries, got %d", len(ops), len(all))
	}
	for i, e := range all {
		if e.Op != ops[i] {
			t.Errorf("entry[%d]: want op %s, got %s", i, ops[i], e.Op)
		}
	}
}

func TestLargeDataPayload(t *testing.T) {
	w, _ := tempWAL(t)

	// 1 KB value
	bigVal := string(make([]byte, 1024))
	e := &wal.Entry{
		Op:    wal.OpInsert,
		Table: "bigdata",
		Data:  map[string]any{"blob": bigVal},
	}
	if err := w.Append(e); err != nil {
		t.Fatalf("append large entry: %v", err)
	}

	all, err := w.ReadAll()
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("want 1 entry, got %d", len(all))
	}
	blob, ok := all[0].Data["blob"].(string)
	if !ok || len(blob) != 1024 {
		t.Errorf("blob: want 1024 bytes, got %d", len(blob))
	}
}

func TestEmptyWALSinceReturnsEmpty(t *testing.T) {
	w, _ := tempWAL(t)
	since, err := w.Since(0)
	if err != nil {
		t.Fatalf("since on empty wal: %v", err)
	}
	if len(since) != 0 {
		t.Errorf("want 0 entries from empty wal, got %d", len(since))
	}
}

func TestWALFileCreatedOnDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "check.wal")

	w, err := wal.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer w.Close()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("WAL file was not created on disk")
	}
}