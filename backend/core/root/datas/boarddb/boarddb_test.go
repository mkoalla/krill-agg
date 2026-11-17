package boarddb

import (
	"krill-agg/root/datas/sqlite"
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
	if err := db.Replace("g1", "best", "all", rows, at); err != nil {
		t.Fatal(err)
	}
	if err := db.Replace("g2", "best", "all", rows[:1], at); err != nil {
		t.Fatal(err)
	}
	// a rebuild replaces only that guild's rows
	if err := db.Replace("g1", "best", "all", rows[1:], at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := db.All("g1", "best", "all")
	if err != nil || len(got) != 2 || got[0].UserID != "b" || got[0].Rank != 1 || got[1].Emojis != rows[2].Emojis {
		t.Fatalf("All(g1) = %+v %v", got, err)
	}
	if other, _ := db.All("g2", "best", "all"); len(other) != 1 {
		t.Fatalf("g2 lost rows: %+v", other)
	}
	page, err := db.Range("g1", "best", "all", 2, 2)
	if err != nil || len(page) != 1 || page[0].UserID != "c" || page[0].Day != 2 {
		t.Fatalf("Range = %+v %v", page, err)
	}
	f, ok, err := db.LastFetch("g1", "best", "all")
	if err != nil || !ok || f.Rows != 2 || !f.At.Equal(at.Add(time.Hour)) {
		t.Fatalf("LastFetch = %+v %v %v", f, ok, err)
	}
	if _, ok, _ := db.LastFetch("g1", "played", "all"); ok {
		t.Fatal("never-built board has a fetch")
	}

	// no day/emojis on the count boards
	if err := db.Replace("g1", "played", "all", []Row{{UserID: "a", Value: 12}}, at); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.All("g1", "played", "all"); got[0].Emojis != "" || got[0].Value != 12 {
		t.Fatalf("played = %+v", got)
	}
	if err := db.Replace("g1", "best; DROP TABLE best", "all", nil, at); err == nil {
		t.Fatal("unknown board accepted")
	}
}

func TestPeriodsKeptApart(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "leaderboard.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint
	at := time.Unix(1_800_000_000, 0)
	db.Replace("g", "best", "all", []Row{{UserID: "a", Value: 500}, {UserID: "b", Value: 400}}, at) //nolint
	db.Replace("g", "best", "week", []Row{{UserID: "b", Value: 400}}, at)                           //nolint
	if all, _ := db.All("g", "best", "all"); len(all) != 2 {
		t.Errorf("all-time = %+v", all)
	}
	if week, _ := db.All("g", "best", "week"); len(week) != 1 || week[0].UserID != "b" || week[0].Rank != 1 {
		t.Errorf("week = %+v", week)
	}
	if _, ok, _ := db.LastFetch("g", "best", "month"); ok {
		t.Error("an unbuilt period has a fetch")
	}
}

// a leaderboard.db from before periods: its boards are dropped (they're a
// cache), the blacklist survives
func TestDropsUnperiodizedBoards(t *testing.T) {
	path := filepath.Join(t.TempDir(), "leaderboard.db")
	old, err := sqlite.New(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE best (guild_id TEXT, rank INTEGER, user_id TEXT, value REAL, day INTEGER, emojis TEXT, PRIMARY KEY (guild_id, rank)) WITHOUT ROWID;`,
		`INSERT INTO best VALUES ('g', 1, 'a', 500, 1, 'x');`,
		`CREATE TABLE fetches (guild_id TEXT, board TEXT, fetched_at INTEGER, rows INTEGER, PRIMARY KEY (guild_id, board)) WITHOUT ROWID;`,
		`CREATE TABLE blacklist (guild_id TEXT, user_id TEXT, PRIMARY KEY (guild_id, user_id)) WITHOUT ROWID;`,
		`INSERT INTO blacklist VALUES ('g', 'z');`,
	} {
		if _, err := old.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	old.Close() //nolint

	db, err := New(path)
	if err != nil {
		t.Fatalf("New() on an old file: %v", err)
	}
	defer db.Close() //nolint
	if rows, err := db.All("g", "best", "all"); err != nil || len(rows) != 0 {
		t.Errorf("old board survived: %+v %v", rows, err)
	}
	if ids, _ := db.Blacklist("g"); len(ids) != 1 || ids[0] != "z" {
		t.Errorf("blacklist lost: %v", ids)
	}
}
