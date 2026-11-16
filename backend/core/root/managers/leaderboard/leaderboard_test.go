package leaderboard

import (
	"krill-agg/root/datas/boarddb"
	"krill-agg/root/krillion"
	"krill-agg/root/managers/scores"
	"path/filepath"
	"testing"
	"time"
)

type fakeSource struct {
	entries  []scores.Entry
	scans    int
	excluded []string // what the last query was asked to leave out
}

func (f *fakeSource) Guild(_ string, exclude []string) ([]scores.Entry, error) {
	f.scans++
	f.excluded = exclude
	return f.entries, nil // ignores exclude, so the Go-side check is what's tested
}

func (f *fakeSource) Players(_ string, exclude []string) ([]string, error) {
	f.excluded = exclude
	seen := map[string]bool{}
	var ids []string
	for _, e := range f.entries {
		if !seen[e.UserID] {
			seen[e.UserID] = true
			ids = append(ids, e.UserID)
		}
	}
	return ids, nil
}

func game(t *testing.T, user string, day int, emojis string) scores.Entry {
	t.Helper()
	p, err := krillion.Rebuild(day, emojis)
	if err != nil {
		t.Fatal(err)
	}
	return scores.Entry{UserID: user, Score: p}
}

// a: 415 (day 1), 0 (day 2)      -> best 415, worst 0, avg 207.5, 2 games
// b: 415 (day 3)                 -> best 415 (later), worst 415, avg 415, 1 game
// c: 210 (day 1), 210 (day 2), 210 (day 3) -> avg 210, 3 games
func fixture(t *testing.T) []scores.Entry {
	return []scores.Entry{
		game(t, "a", 1, "🦑🦑🦑🐟🦑🦑🏮"),
		game(t, "a", 2, "⬛⬛⬛⬛⬛⬛⬛"),
		game(t, "b", 3, "🦑🦑🦑🐟🦑🦑🏮"),
		game(t, "c", 1, "🐟🐟🐟⬛🐟🦑🐟"),
		game(t, "c", 2, "🐟🐟🐟⬛🐟🦑🐟"),
		game(t, "c", 3, "🐟🐟🐟⬛🐟🦑🐟"),
	}
}

func users(rows []Row) []string {
	out := make([]string, len(rows))
	for j, r := range rows {
		out[j] = r.UserID
	}
	return out
}

func TestBuild(t *testing.T) {
	entries := fixture(t)
	cases := map[string][]string{
		BEST:    {"a", "b", "c"}, // a and b tie on 415; a got it first
		WORST:   {"a", "c", "b"},
		AVERAGE: {"b", "c", "a"},
		PLAYED:  {"c", "a", "b"},
	}
	for board, want := range cases {
		rows := Build(board, entries)
		if got := users(rows); len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
			t.Errorf("%s order = %v, want %v", board, got, want)
		}
		if rows[0].Rank != 1 || rows[2].Rank != 3 {
			t.Errorf("%s ranks = %d..%d", board, rows[0].Rank, rows[2].Rank)
		}
	}
	best := Build(BEST, entries)[0]
	if best.Value != 415 || best.Day != 1 || best.Emojis != "🦑🦑🦑🐟🦑🦑🏮" {
		t.Errorf("best row = %+v", best)
	}
	if avg := Build(AVERAGE, entries)[2]; avg.Value != 207.5 || avg.Emojis != "" {
		t.Errorf("average row = %+v", avg)
	}
	if Build("nope", entries) != nil {
		t.Error("unknown board built")
	}
}

func TestLazyHourlyCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "leaderboard.db")
	db, err := boarddb.New(path)
	if err != nil {
		t.Fatal(err)
	}
	src := &fakeSource{entries: fixture(t)}
	man := newManager(db, src)
	clock := time.Date(2026, 11, 16, 15, 10, 0, 0, time.UTC)
	man.now = func() time.Time { return clock }

	if n, _ := man.Players("g"); n != 3 || src.scans != 0 {
		t.Fatalf("Players() = %d after %d scans; should count without building", n, src.scans)
	}
	if _, _, err := man.Board("g", BEST); err != nil || src.scans != 1 {
		t.Fatalf("first Board: %v, %d scans", err, src.scans)
	}
	man.Board("g", BEST) //nolint
	if src.scans != 1 {
		t.Fatal("same-hour Board rebuilt")
	}
	man.Board("g", PLAYED) //nolint
	if src.scans != 2 {
		t.Fatal("each board is built on its own")
	}

	// a restart within the hour reads the db instead of rebuilding
	man.Close() //nolint
	db, _ = boarddb.New(path)
	man = newManager(db, src)
	man.now = func() time.Time { return clock }
	defer man.Close() //nolint
	rows, at, err := man.Board("g", BEST)
	if err != nil || src.scans != 2 || len(rows) != 3 || rows[0].Rank != 1 || !at.Equal(clock.Truncate(time.Second)) {
		t.Fatalf("after restart: %d rows, at %v, %d scans, %v", len(rows), at, src.scans, err)
	}

	// next hour: stale, rebuilt
	clock = clock.Add(time.Hour)
	man.Board("g", BEST) //nolint
	if src.scans != 3 {
		t.Fatalf("stale board not rebuilt (%d scans)", src.scans)
	}

	// refetch rebuilds everything at once
	if err := man.Refetch("g"); err != nil || src.scans != 7 {
		t.Fatalf("Refetch: %v, %d scans", err, src.scans)
	}
	if NextUpdate(clock) != time.Date(2026, 11, 16, 17, 0, 0, 0, time.UTC) {
		t.Errorf("NextUpdate = %v", NextUpdate(clock))
	}
}

func TestBlacklist(t *testing.T) {
	db, err := boarddb.New(filepath.Join(t.TempDir(), "leaderboard.db"))
	if err != nil {
		t.Fatal(err)
	}
	src := &fakeSource{entries: fixture(t)}
	man := newManager(db, src)
	defer man.Close() //nolint

	man.Board("g", BEST) //nolint
	if changed, err := man.SetBlacklisted("g", "a", true); !changed || err != nil {
		t.Fatalf("add: %v %v", changed, err)
	}
	if changed, _ := man.SetBlacklisted("g", "a", true); changed {
		t.Error("adding twice reported a change")
	}
	if n, _ := man.Players("g"); n != 2 {
		t.Errorf("Players() = %d with a blacklisted, want 2", n)
	}
	// the cached board was dropped, so this rebuilds without a
	rows, _, _ := man.Board("g", BEST)
	if len(src.excluded) != 1 || src.excluded[0] != "a" {
		t.Errorf("query wasn't asked to exclude a: %v", src.excluded)
	}
	if got := users(rows); len(got) != 2 || got[0] != "b" || rows[0].Rank != 1 {
		t.Errorf("best without a = %v", got)
	}
	if ids, _ := man.Blacklist("g"); len(ids) != 1 || ids[0] != "a" {
		t.Errorf("Blacklist() = %v", ids)
	}
	if on, _ := man.IsBlacklisted("other", "a"); on {
		t.Error("blacklist leaked across guilds")
	}

	// survives a restart (read back from the db), and removal brings them back
	man2 := newManager(db, src)
	if on, _ := man2.IsBlacklisted("g", "a"); !on {
		t.Error("blacklist not persisted")
	}
	if changed, _ := man2.SetBlacklisted("g", "a", false); !changed {
		t.Error("remove reported no change")
	}
	if rows, _, _ := man2.Board("g", BEST); len(rows) != 3 {
		t.Errorf("after removal %d rows, want 3", len(rows))
	}
}
