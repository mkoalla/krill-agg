package tracking

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestTrackPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracking.db")
	man, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if n, err := man.Track("1", "100", "101"); n != 2 || err != nil {
		t.Fatalf("Track() = %d %v", n, err)
	}
	if n, _ := man.Track("1", "100"); n != 0 {
		t.Fatalf("re-Track() = %d, want 0", n)
	}
	if n, _ := man.Track("2", "200"); n != 1 {
		t.Fatalf("Track(guild 2) = %d, want 1", n)
	}
	if n, _ := man.Untrack("1", "100"); n != 1 {
		t.Fatalf("Untrack() = %d, want 1", n)
	}
	man.Close() //nolint

	man, err = New(path)
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	defer man.Close() //nolint
	if got := man.Channels("1"); !slices.Equal(got, []string{"101"}) {
		t.Fatalf("Channels(1) = %v, want [101]", got)
	}
	if !man.IsTracked("2", "200") || man.IsTracked("1", "100") {
		t.Fatal("IsTracked mismatch after reopen")
	}
	if n, _ := man.UntrackAll("1"); n != 1 || len(man.Channels("1")) != 0 {
		t.Fatalf("UntrackAll() = %d, left %v", n, man.Channels("1"))
	}
}
