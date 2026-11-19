package top

import (
	"errors"
	"krill-agg/root/info/dailygen"
	"krill-agg/root/info/krillionapi"
	"krill-agg/root/managers/scores"
	"krill-agg/root/types"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMAWindow(t *testing.T) {
	for _, c := range []struct {
		span   int
		window int
		label  string
	}{
		{1, 3, "3-DAY"},
		{7, 3, "3-DAY"},
		{18, 3, "3-DAY"},  // a tie between a week and a month goes shorter
		{19, 7, "1-WEEK"}, // nearer a month
		{30, 7, "1-WEEK"},
		{60, 7, "1-WEEK"},
		{61, 14, "2-WEEK"},
		{91, 14, "2-WEEK"},
		{137, 30, "1-MONTH"},
		{182, 30, "1-MONTH"},
		{900, 30, "1-MONTH"}, // never cumulative
	} {
		if w, l := maWindow(c.span); w != c.window || l != c.label {
			t.Errorf("maWindow(%d) = %d %q, want %d %q", c.span, w, l, c.window, c.label)
		}
	}
}

func games(dayScores ...int) []scores.PlayerScore {
	var out []scores.PlayerScore
	for j := 0; j < len(dayScores); j += 2 {
		out = append(out, scores.PlayerScore{Day: dayScores[j], Score: dayScores[j+1]})
	}
	return out
}

func TestMovingAverage(t *testing.T) {
	// 3-day window: day 4 counts 2-4, day 8 only itself (5-7 off)
	got := movingAverage(games(1, 100, 2, 200, 3, 300, 4, 400, 8, 800), 3)
	want := []float64{100, 150, 200, 300, 800}
	for j, a := range got {
		if a.Value != want[j] || a.Day != games(1, 0, 2, 0, 3, 0, 4, 0, 8, 0)[j].Day {
			t.Errorf("ma[%d] = %+v, want %v", j, a, want[j])
		}
	}
	if len(movingAverage(nil, 3)) != 0 {
		t.Error("no games, no average")
	}
}

func TestHistory(t *testing.T) {
	entries := []scores.Entry{
		{UserID: "a", Score: scores.PlayerScore{Day: 5, Score: 1}},
		{UserID: "b", Score: scores.PlayerScore{Day: 4, Score: 2}},
		{UserID: "a", Score: scores.PlayerScore{Day: 2, Score: 3}},
	}
	h := history(entries, "a")
	if len(h) != 2 || h[0].Day != 2 || h[1].Day != 5 {
		t.Errorf("history = %+v", h)
	}
}

func TestTrendName(t *testing.T) {
	if got := trendName("week", games(118, 1, 120, 2, 124, 3), nil); got != "trend_week_118-124_g0.webp" {
		t.Errorf("trendName = %q", got)
	}
	// a day's histogram saved later is a new card
	global := []dailygen.TrendAverage{{Day: 118, Value: 300}, {Day: 123, Value: 310}}
	if got := trendName("week", games(118, 1, 124, 3), global); got != "trend_week_118-124_g123.webp" {
		t.Errorf("trendName with global = %q", got)
	}
}

// a card already on disk for the same games is reused: nothing is drawn
func TestCachedTrendReuses(t *testing.T) {
	bcp := &types.BotCommandPackage{DataDir: t.TempDir()}
	g := games(118, 300, 124, 400)
	path := trendPath(bcp, "1", "2", "week", g, nil)
	if want := filepath.Join(bcp.DataDir, dailygen.DIR, "1", "users", "2", "trend_week_118-124_g0.webp"); path != want {
		t.Fatalf("trendPath = %q, want %q", path, want)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("card"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := cachedTrend(nil, bcp, path, "1", "Krill Club", "2", "week", g, nil, func() (string, error) {
		t.Error("drew a card that was already on disk")
		return "", nil
	})
	if err != nil {
		t.Errorf("cachedTrend: %v", err)
	}
	if got := trendLink(path); got != strings.TrimSuffix(path, "webp")+"link" {
		t.Errorf("trendLink = %q", got)
	}
}

// a second draw for the same player (any period) is turned away with the
// first's link; other players, and the same one once it's done, go ahead
func TestClaimDraw(t *testing.T) {
	publish, done, err := claimDraw("1", "2")
	if err != nil {
		t.Fatal(err)
	}
	var drawing *drawingError
	if _, _, err := claimDraw("1", "2"); !errors.As(err, &drawing) || drawing.link != "" {
		t.Fatalf("second claim before the reply: %v", err)
	}
	publish("https://discord.com/channels/1/3/4")
	if _, _, err := claimDraw("1", "2"); !errors.As(err, &drawing) || drawing.link != "https://discord.com/channels/1/3/4" {
		t.Fatalf("second claim after the reply: %v", err)
	}
	_, other, err := claimDraw("1", "5")
	if err != nil {
		t.Fatalf("another player was turned away: %v", err)
	}
	other()
	done()
	if _, again, err := claimDraw("1", "2"); err != nil {
		t.Fatalf("claim after done: %v", err)
	} else {
		again()
	}
}

// a card missing from disk while the player's draw is underway isn't drawn
func TestCachedTrendTurnsAwayADuplicate(t *testing.T) {
	bcp := &types.BotCommandPackage{DataDir: t.TempDir()}
	g := games(118, 300, 124, 400)
	_, done, err := claimDraw("1", "2")
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	err = cachedTrend(nil, bcp, trendPath(bcp, "1", "2", "month", g, nil), "1", "Krill Club", "2", "month", g, nil, func() (string, error) {
		t.Error("drew while another draw was underway")
		return "", nil
	})
	var drawing *drawingError
	if !errors.As(err, &drawing) {
		t.Errorf("cachedTrend = %v, want a *drawingError", err)
	}
}

// clears one player's folder or every player's, skipping one mid-draw
func TestClearTrends(t *testing.T) {
	bcp := &types.BotCommandPackage{DataDir: t.TempDir()}
	card := func(user, name string) {
		path := filepath.Join(trendsDir(bcp, "1"), user, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	card("a", "trend_week_1-3.webp")
	card("a", "trend_week_1-3.link")
	card("a", "trend_all_1-9.webp")
	card("b", "trend_week_2-4.webp")
	card("c", "trend_week_2-4.webp")

	if n, busy, err := clearTrends(bcp, "1", []string{"a"}); err != nil || n != 2 || len(busy) != 0 {
		t.Fatalf("clear a = %d %v %v", n, busy, err)
	}
	if _, err := os.Stat(filepath.Join(trendsDir(bcp, "1"), "a")); !os.IsNotExist(err) {
		t.Errorf("a's folder still there: %v", err)
	}

	_, done, err := claimDraw("1", "c") // c's card is being drawn
	if err != nil {
		t.Fatal(err)
	}
	n, busy, err := clearTrends(bcp, "1", nil)
	done()
	if err != nil || n != 1 || len(busy) != 1 || busy[0] != "c" {
		t.Fatalf("clear all = %d %v %v", n, busy, err)
	}
	if _, err := os.Stat(filepath.Join(trendsDir(bcp, "1"), "c", "trend_week_2-4.webp")); err != nil {
		t.Errorf("c's card was removed mid-draw: %v", err)
	}

	// a guild nobody has cards in yet
	if n, busy, err := clearTrends(bcp, "2", nil); err != nil || n != 0 || len(busy) != 0 {
		t.Errorf("empty guild = %d %v %v", n, busy, err)
	}
}

// every player's median on each day the games span that has a histogram
func TestGlobalTrend(t *testing.T) {
	saved := map[int]krillionapi.Stats{
		99:  hist(10, 100), // before their first game
		101: hist(10, 100), // 350-385: 367.5
		102: {Sample: 0},   // no histogram
		104: hist(0, 50),   // 0-35: 17.5
	}
	dist := func(day int) (krillionapi.Stats, bool) {
		s, ok := saved[day]
		return s, ok
	}
	got := globalTrend(dist, games(100, 300, 104, 400))
	want := []dailygen.TrendAverage{{Day: 101, Value: 367.5}, {Day: 104, Value: 17.5}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("globalTrend = %+v", got)
	}
}
