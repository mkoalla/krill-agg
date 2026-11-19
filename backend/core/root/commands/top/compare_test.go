package top

import (
	"krill-agg/root/info/dailygen"
	"krill-agg/root/managers/leaderboard"
	"krill-agg/root/managers/scores"
	"krill-agg/root/types"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPairOf(t *testing.T) {
	for _, c := range []struct {
		caller, u1, u2 string
		left, right    string
		ok             bool
	}{
		{"5", "", "", "5", GUILD_SIDE, true}, // nobody: the caller against the guild
		{"5", "3", "", "3", "5", true},       // one: the caller against them, lower id left
		{"5", "", "7", "5", "7", true},       // either slot
		{"5", "9", "12", "12", "9", true},    // as strings: "12" < "9"
		{"5", "7", "3", "3", "7", true},      // order given doesn't matter
		{"5", "5", "", "", "", false},        // themselves
		{"5", "7", "7", "", "", false},       // someone twice
	} {
		left, right, ok := pairOf(c.caller, c.u1, c.u2)
		if left != c.left || right != c.right || ok != c.ok {
			t.Errorf("pairOf(%q, %q, %q) = %q %q %v, want %q %q %v", c.caller, c.u1, c.u2, left, right, ok, c.left, c.right, c.ok)
		}
	}
}

func TestRowWinner(t *testing.T) {
	r1, r2, r2b := &Row{Rank: 1}, &Row{Rank: 2}, &Row{Rank: 2}
	for _, c := range []struct {
		l, r   *Row
		lw, rw bool
	}{
		{r1, r2, true, false},
		{r2, r1, false, true},
		{r2, r2b, false, false}, // a tie: nobody
		{r2, nil, true, false},  // only one ranked
		{nil, r1, false, true},
		{nil, nil, false, false},
	} {
		if lw, rw := rowWinner(c.l, c.r); lw != c.lw || rw != c.rw {
			t.Errorf("rowWinner(%v, %v) = %v %v", c.l, c.r, lw, rw)
		}
	}
}

func boards() boardRows {
	return boardRows{
		leaderboard.BEST:    {{UserID: "a", Rank: 1, Value: 600, Emojis: "🦑", Day: 9}, {UserID: "b", Rank: 2, Value: 500}},
		leaderboard.WORST:   {{UserID: "b", Rank: 1, Value: 100}, {UserID: "a", Rank: 2, Value: 200}},
		leaderboard.AVERAGE: {{UserID: "a", Rank: 1, Value: 412.25}},
		leaderboard.PLAYED:  {},
	}
}

func TestVersusFields(t *testing.T) {
	f := versusFields(boards(), "a", "b", "Trick")
	if len(f) != 3 {
		t.Fatalf("%d fields", len(f))
	}
	names, left, right := strings.Split(f[0].Value, "\n"), strings.Split(f[1].Value, "\n"), strings.Split(f[2].Value, "\n")
	if left[0] != "<@a>" || right[0] != "<@b>" || names[1] != "**Best Scores**" {
		t.Errorf("headers %q %q %q", names[0], left[0], right[0])
	}
	// "#rank. value", no emojis; the better rank gets the arm
	want := [][2]string{
		{"💪 `#1. 600`", "`#2. 500`"},
		{"`#2. 200`", "💪 `#1. 100`"},
		{"💪 `#1. 412.2`", "`—`"}, // only one ranked
		{"`—`", "`—`"},           // neither
	}
	for j, w := range want {
		if left[j+1] != w[0] || right[j+1] != w[1] {
			t.Errorf("row %d = %q | %q, want %q | %q", j, left[j+1], right[j+1], w[0], w[1])
		}
	}
}

// against the guild: its column is headed by its name and blank, and the
// player takes every row they're on
func TestVersusFieldsAgainstTheGuild(t *testing.T) {
	f := versusFields(boards(), "b", GUILD_SIDE, "Trick")
	if len(f) != 3 {
		t.Fatalf("%d fields", len(f))
	}
	col, guild := strings.Split(f[1].Value, "\n"), strings.Split(f[2].Value, "\n")
	if col[0] != "<@b>" || col[1] != "💪 `#2. 500`" || col[2] != "💪 `#1. 100`" || col[3] != "`—`" {
		t.Errorf("player column = %q", col)
	}
	if guild[0] != "Trick" || len(guild) != 5 || guild[1] != "`-`" || guild[4] != "`-`" {
		t.Errorf("guild column = %q", guild)
	}
}

func TestVersusTitle(t *testing.T) {
	if got := versusTitle("alice", "Krill Club"); got != "alice vs Krill Club" {
		t.Errorf("title = %q", got)
	}
	if periodNote(leaderboard.ALLTIME) != "" || periodNote(leaderboard.MONTH) != "-# 1 Month" {
		t.Errorf("period notes %q %q", periodNote(leaderboard.ALLTIME), periodNote(leaderboard.MONTH))
	}
}

func TestSides(t *testing.T) {
	entries := []scores.Entry{
		entry("a", 98, 700), // before the period
		entry("a", 100, 300), entry("b", 100, 400), entry("x", 100, 50),
		entry("a", 103, 500), entry("b", 103, 100),
		entry("a", 110, 650), // today: not finished
	}
	a := playerSide(entries, "a", 99, 109)
	if len(a.Points) != 2 || a.Dives != 2 || *a.Average != 400 || a.Guild {
		t.Errorf("a = %+v", a)
	}
	g := guildSide(entries, map[string]bool{"x": true}, 99, 109)
	want := []dailygen.TrendAverage{{Day: 100, Value: 350}, {Day: 103, Value: 300}}
	if len(g.Points) != 2 || g.Points[0] != want[0] || g.Points[1] != want[1] || g.Dives != 4 || *g.Average != 325 || !g.Guild {
		t.Errorf("guild = %+v", g)
	}
	nobody := playerSide(entries, "z", 99, 109)
	if nobody.Average != nil || nobody.Dives != 0 {
		t.Errorf("nobody = %+v", nobody)
	}

	first, window, ok := compareSpan(&a, &g, 109)
	if !ok || first != 100 || window != "3-DAY" || len(a.MA) != 2 || len(g.MA) != 2 {
		t.Errorf("span = %d %q %v", first, window, ok)
	}
	if _, _, ok := compareSpan(&nobody, &dailygen.CompareSide{}, 109); ok {
		t.Error("a span with no games on either side")
	}
}

// a card a day per pair and period
func TestComparePath(t *testing.T) {
	bcp := &types.BotCommandPackage{DataDir: "/data"}
	path := comparePath(bcp, "1", "12", "9", leaderboard.MONTH, 130)
	if path != filepath.Join("/data", "dailygen", "1", "compare", "compare_12_9_month_130.webp") {
		t.Errorf("comparePath = %q", path)
	}
	if !strings.HasPrefix(filepath.Base(path), comparePrefix("12", "9", leaderboard.MONTH)+"_") {
		t.Error("the stale glob would miss it")
	}
}

// clears one player's pairs or every pair, skipping one mid-draw
func TestClearCompares(t *testing.T) {
	bcp := &types.BotCommandPackage{DataDir: t.TempDir()}
	dir := compareDir(bcp, "1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"compare_10_20_month_130.webp", "compare_10_20_month_130.link", "compare_10_20_all_130.webp",
		"compare_10_server_all_130.webp", "compare_20_30_all_130.webp", "compare_30_40_all_130.webp",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(dir, name))
		return err == nil
	}

	// 10's pairs: with 20 (two cards and a link) and with the server
	if n, busy, err := clearCompares(bcp, "1", "10"); err != nil || n != 3 || len(busy) != 0 {
		t.Fatalf("clear 10 = %d %v %v", n, busy, err)
	}
	if exists("compare_10_20_month_130.link") || exists("compare_10_server_all_130.webp") || !exists("compare_20_30_all_130.webp") {
		t.Error("cleared the wrong cards")
	}

	_, done, err := claimDraw("1", "compare-30-40") // being drawn
	if err != nil {
		t.Fatal(err)
	}
	n, busy, err := clearCompares(bcp, "1", "")
	done()
	if err != nil || n != 1 || len(busy) != 1 || busy[0] != [2]string{"30", "40"} {
		t.Fatalf("clear all = %d %v %v", n, busy, err)
	}
	if !exists("compare_30_40_all_130.webp") {
		t.Error("a card was removed mid-draw")
	}
	// a guild with none yet
	if n, busy, err := clearCompares(bcp, "2", ""); err != nil || n != 0 || len(busy) != 0 {
		t.Errorf("empty guild = %d %v %v", n, busy, err)
	}
}
