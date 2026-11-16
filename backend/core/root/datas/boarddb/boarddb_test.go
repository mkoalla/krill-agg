package boarddb

import (
	"path/filepath"
	"testing"
	"time"
)

func TestReplaceAndRange(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "leaderboard.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint
	at := time.Unix(1_800_000_000, 0)
	rows := []Row{
		{UserID: "a", Value: 500, Day: 3, Emojis: "🌟🌟🌟🌟🌟⬛⬛"},
		{UserID: "b", Value: 400, Day: 1, Emojis: "🦑🦑🦑🦑🦑🌟⬛"},
		{UserID: "c", Value: 300, Day: 2, Emojis: "🌟🌟🌟⬛⬛⬛⬛"},
	}
	if err := db.Replace("g1", "best", rows, at); err != nil {
		t.Fatal(err)
	}
	if err := db.Replace("g2", "best", rows[:1], at); err != nil {
		t.Fatal(err)
	}
	// a rebuild replaces only that guild's rows
	if err := db.Replace("g1", "best", rows[1:], at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := db.All("g1", "best")
	if err != nil || len(got) != 2 || got[0].UserID != "b" || got[0].Rank != 1 || got[1].Emojis != rows[2].Emojis {
		t.Fatalf("All(g1) = %+v %v", got, err)
	}
	if other, _ := db.All("g2", "best"); len(other) != 1 {
		t.Fatalf("g2 lost rows: %+v", other)
	}
	page, err := db.Range("g1", "best", 2, 2)
	if err != nil || len(page) != 1 || page[0].UserID != "c" || page[0].Day != 2 {
		t.Fatalf("Range = %+v %v", page, err)
	}
	f, ok, err := db.LastFetch("g1", "best")
	if err != nil || !ok || f.Rows != 2 || !f.At.Equal(at.Add(time.Hour)) {
		t.Fatalf("LastFetch = %+v %v %v", f, ok, err)
	}
	if _, ok, _ := db.LastFetch("g1", "played"); ok {
		t.Fatal("never-built board has a fetch")
	}

	// no day/emojis on the count boards
	if err := db.Replace("g1", "played", []Row{{UserID: "a", Value: 12}}, at); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.All("g1", "played"); got[0].Emojis != "" || got[0].Value != 12 {
		t.Fatalf("played = %+v", got)
	}
	if err := db.Replace("g1", "best; DROP TABLE best", nil, at); err == nil {
		t.Fatal("unknown board accepted")
	}
}
