package filedict

import (
	"path/filepath"
	"strconv"
	"testing"
)

func anyEntry(*string, *int) bool { return true }

func open(t *testing.T, path string) *FileDict[string, int] {
	t.Helper()
	table, err := New(path, anyEntry)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return table
}

// SetSync/DropSync hit the file on their own; Close flushes memory-only Set
func TestPartialWritesAndCloseFlush(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dict.db")
	table := open(t, path)
	if err := table.SetSync("a", 1); err != nil {
		t.Fatal(err)
	}
	if err := table.SetSync("b", 2); err != nil {
		t.Fatal(err)
	}
	if err := table.DropSync("a"); err != nil {
		t.Fatal(err)
	}

	// a second handle sees the partial writes without any SyncWrite/Close
	other := open(t, path)
	if _, ok := other.Get("a"); ok {
		t.Error("dropped key still on file")
	}
	if v, ok := other.Get("b"); !ok || v != 2 {
		t.Errorf("Get(b) = %v %v, want 2", v, ok)
	}
	other.file.Close() //nolint (skip the flush; this handle is only for reading)

	if err := table.Set("c", 3); err != nil {
		t.Fatal(err)
	}
	if err := table.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	table = open(t, path)
	defer table.Close() //nolint
	if v, ok := table.Get("c"); !ok || v != 3 {
		t.Errorf("Get(c) after reopen = %v %v, want 3 (flushed on close)", v, ok)
	}
}

// more rows than fit in one statement's parameter cap
func TestSyncWriteChunks(t *testing.T) {
	if raceEnabled {
		// 20000 rows run past SyncWrite's own deadline once -race slows them down
		t.Skip("WARNING: disabled under -race (too slow); run without -race to cover chunked writes")
	}
	path := filepath.Join(t.TempDir(), "dict.db")
	table := open(t, path)
	const n = 20000 // 40000 params > 32766
	for i := range n {
		table.Set(strconv.Itoa(i), i) //nolint
	}
	if err := table.SyncWrite(); err != nil {
		t.Fatalf("SyncWrite() error = %v", err)
	}
	table.Close() //nolint

	table = open(t, path)
	defer table.Close() //nolint
	if len(table.data) != n {
		t.Fatalf("reopened with %d entries, want %d", len(table.data), n)
	}
}
