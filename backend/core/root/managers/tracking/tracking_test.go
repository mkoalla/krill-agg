package tracking

import (
	"encoding/json"
	"krill-agg/root/datas/sqlite"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestTrackPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracking.db")
	man, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if n, main, err := man.Track("1", "100", "101"); n != 2 || main != "100" || err != nil {
		t.Fatalf("Track() = %d %q %v, want the first channel made main", n, main, err)
	}
	if n, _, _ := man.Track("1", "100"); n != 0 {
		t.Fatalf("re-Track() = %d, want 0", n)
	}
	if n, main, _ := man.Track("1", "102"); n != 1 || main != "" {
		t.Fatalf("Track(more) = %d %q, main shouldn't move", n, main)
	}
	if n, _, _ := man.Track("2", "200"); n != 1 {
		t.Fatalf("Track(guild 2) = %d, want 1", n)
	}
	if n, lost, _ := man.Untrack("1", "101"); n != 1 || lost {
		t.Fatalf("Untrack() = %d %v", n, lost)
	}
	man.Close() //nolint

	man, err = New(path)
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	defer man.Close() //nolint
	if got := man.Channels("1"); !slices.Equal(got, []string{"100", "102"}) {
		t.Fatalf("Channels(1) = %v, want [100 102]", got)
	}
	if main, _ := man.Main("1"); main != "100" {
		t.Fatalf("Main(1) = %q after reopen", main)
	}
	if !man.IsTracked("2", "200") || man.IsTracked("1", "101") {
		t.Fatal("IsTracked mismatch after reopen")
	}
	if n, _ := man.UntrackAll("1"); n != 2 || len(man.Channels("1")) != 0 {
		t.Fatalf("UntrackAll() = %d, left %v", n, man.Channels("1"))
	}
	if _, ok := man.Main("1"); ok {
		t.Fatal("UntrackAll should clear the main channel")
	}
}

func TestMainChannelCascade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracking.db")
	man, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer man.Close()            //nolint
	man.Track("1", "100", "101") //nolint

	old, newly, err := man.SetMain("1", "101")
	if err != nil || old != "100" || newly {
		t.Fatalf("SetMain(tracked) = %q %v %v", old, newly, err)
	}
	old, newly, err = man.SetMain("1", "300")
	if err != nil || old != "101" || !newly || !man.IsTracked("1", "300") {
		t.Fatalf("SetMain(untracked) = %q %v %v, should track it", old, newly, err)
	}
	if got := man.Mains(); got["1"] != "300" {
		t.Fatalf("Mains() = %v", got)
	}

	// the file's own cascade, not just the in-memory mirror
	if _, lost, _ := man.Untrack("1", "300"); !lost {
		t.Fatal("untracking the main channel should report it")
	}
	if _, ok, err := man.db.Main("1"); ok || err != nil {
		t.Fatalf("db main survived untrack: %v %v", ok, err)
	}
	if _, ok := man.Main("1"); ok {
		t.Fatal("memory main survived untrack")
	}
	// nothing is main now, but channels are still tracked: no automatic main
	if _, main, _ := man.Track("1", "400"); main != "" {
		t.Fatalf("Track() made %q main with channels already tracked", main)
	}
}

// the filedict format tracking used before: data(key, value json array)
func TestMigratesFileDict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracking.db")
	old, err := sqlite.New(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal([]string{"100", "101"})
	if _, err := old.Exec(`CREATE TABLE data (key PRIMARY KEY, value BLOB NOT NULL);`); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`INSERT INTO data (key, value) VALUES (?, ?);`, "1", raw); err != nil {
		t.Fatal(err)
	}
	old.Close() //nolint

	man, err := New(path)
	if err != nil {
		t.Fatalf("New() on old file: %v", err)
	}
	if got := man.Channels("1"); !slices.Equal(got, []string{"100", "101"}) {
		t.Fatalf("migrated Channels(1) = %v", got)
	}
	man.Close() //nolint
	// a second open finds nothing left to migrate
	man, err = New(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer man.Close() //nolint
	if got := man.Channels("1"); !slices.Equal(got, []string{"100", "101"}) {
		t.Fatalf("Channels(1) after reopen = %v", got)
	}
}
