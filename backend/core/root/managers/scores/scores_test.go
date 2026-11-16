package scores

import (
	"path/filepath"
	"testing"
)

const (
	day124 = "Krillion #124 🦐\n415\n\n🦑🦑🦑🐟🦑🦑🏮"
	day123 = "Krillion #123 🦐\n495\n\n🦑🏮🏮🏮🏮🫧🏮"
	alt123 = "Krillion #123 🦐\n310\n\n🏮🫧🫧🏮🐟🦑🐟"
)

func open(t *testing.T, path string) *ScoreManager {
	t.Helper()
	man, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return man
}

func TestIngestFlow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scores.db")
	man := open(t, path)

	if r := man.Ingest("1", "10", "just chatting"); r.Outcome != Ignored {
		t.Fatalf("chat outcome = %v, want Ignored", r.Outcome)
	}
	if r := man.Ingest("1", "10", "Krillion #5\n999\n\n🦑"); r.Outcome != Rejected {
		t.Fatalf("bad outcome = %v, want Rejected", r.Outcome)
	}
	if r := man.Ingest("1", "10", day123); r.Outcome != Stored {
		t.Fatalf("first outcome = %v (%v), want Stored", r.Outcome, r.Err)
	}
	r := man.Ingest("1", "10", alt123)
	if r.Outcome != Conflict || r.Existing.Score != 495 || r.Score.Score != 310 {
		t.Fatalf("second = %+v, want Conflict 495 -> 310", r)
	}
	// same user and day in another guild is independent
	if r := man.Ingest("2", "10", alt123); r.Outcome != Stored {
		t.Fatalf("other guild outcome = %v, want Stored", r.Outcome)
	}
	if err := man.Set("1", "10", r.Score); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if r := man.Ingest("1", "20", day123); r.Outcome != Stored {
		t.Fatalf("second user outcome = %v, want Stored", r.Outcome)
	}
	if r := man.Ingest("1", "20", day124); r.Outcome != Stored {
		t.Fatalf("other day outcome = %v, want Stored", r.Outcome)
	}
	if err := man.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// everything survives a restart, and scores are rebuilt from the emojis
	man = open(t, path)
	defer man.Close() //nolint
	entries, err := man.Day("1", 123)
	if err != nil {
		t.Fatalf("Day() error = %v", err)
	}
	if len(entries) != 2 || entries[0].UserID != "20" || entries[0].Score.Score != 495 ||
		entries[1].UserID != "10" || entries[1].Score.Score != 310 {
		t.Fatalf("Day(1, 123) = %+v, want 20:495 then 10:310", entries)
	}
	if entries, _ := man.Day("1", 999); len(entries) != 0 {
		t.Fatalf("Day(1, 999) = %+v, want empty", entries)
	}
	if p, ok, err := man.Get("2", 123, "10"); err != nil || !ok || p.Score != 310 {
		t.Fatalf("Get(2, 123, 10) = %+v %v %v", p, ok, err)
	}
}

// backfill's bulk write overwrites existing entries and adds new ones atomically
func TestSetMany(t *testing.T) {
	man := open(t, filepath.Join(t.TempDir(), "scores.db"))
	defer man.Close() //nolint
	if r := man.Ingest("1", "10", day123); r.Outcome != Stored {
		t.Fatalf("seed outcome = %v", r.Outcome)
	}
	older := man.Ingest("9", "10", alt123).Score // parsed, stored elsewhere; reuse the struct
	newer := man.Ingest("9", "20", day124).Score
	if err := man.SetMany("1", []Entry{{UserID: "10", Score: older}, {UserID: "20", Score: newer}}); err != nil {
		t.Fatalf("SetMany() error = %v", err)
	}
	if p, _, _ := man.Get("1", 123, "10"); p.Score != 310 {
		t.Errorf("user 10 day 123 = %d, want 310 (overwritten)", p.Score)
	}
	if p, ok, _ := man.Get("1", 124, "20"); !ok || p.Score != 415 {
		t.Errorf("user 20 day 124 = %d %v, want 415", p.Score, ok)
	}
}
