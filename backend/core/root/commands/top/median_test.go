package top

import (
	"errors"
	"krill-agg/root/info/dailygen"
	"krill-agg/root/info/krillionapi"
	"krill-agg/root/managers/leaderboard"
	"krill-agg/root/managers/scores"
	"krill-agg/root/types"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMedianOf(t *testing.T) {
	if got := medianOf([]int{100, 300, 700}); got != 300 {
		t.Errorf("odd = %v", got)
	}
	if got := medianOf([]int{100, 301}); got != 200.5 {
		t.Errorf("even = %v", got)
	}
}

func TestDistMedian(t *testing.T) {
	dist := make([]float64, 20)
	dist[10] = 10 // everyone in 350-385
	if got, ok := distMedian(dist); !ok || got != 350+17.5 {
		t.Errorf("one bucket = %v %v", got, ok)
	}
	dist[2] = 10 // half in 70-105: the middle is the top of that one
	if got, ok := distMedian(dist); !ok || got != 105 {
		t.Errorf("two buckets = %v %v", got, ok)
	}
	if _, ok := distMedian(make([]float64, 20)); ok {
		t.Error("an empty histogram has a median")
	}
	if _, ok := distMedian(nil); ok {
		t.Error("no histogram has a median")
	}
}

func entry(user string, day, score int) scores.Entry {
	return scores.Entry{UserID: user, Score: scores.PlayerScore{Day: day, Score: score}}
}

func hist(bucket int, count float64) krillionapi.Stats {
	dist := make([]float64, 20)
	dist[bucket] = count
	return krillionapi.Stats{Sample: int(count), Dist: dist}
}

func TestBuildSpan(t *testing.T) {
	entries := []scores.Entry{
		entry("a", 98, 700), // before the period
		entry("a", 100, 300), entry("b", 100, 400),
		entry("a", 103, 500), entry("b", 103, 100), entry("c", 103, 200),
		entry("a", 110, 650), // today: not finished
	}
	saved := map[int]krillionapi.Stats{
		101: hist(10, 100),      // 350-385, median 367.5
		103: hist(0, 50),        // 0-35, median 17.5
		104: {Sample: 0},        // no histogram that day
		109: hist(19, 50),       // 665-700, median 682.5
		120: hist(5, 1_000_000), // after the span
	}
	dist := func(day int) (krillionapi.Stats, bool) {
		s, ok := saved[day]
		return s, ok
	}
	span, ok := buildSpan(entries, dist, 99, 109)
	if !ok {
		t.Fatal("no span")
	}
	if span.first != 100 || span.last != 109 || span.players != 3 {
		t.Errorf("span = %d-%d with %d players", span.first, span.last, span.players)
	}
	want := []dailygen.TrendAverage{{Day: 100, Value: 350}, {Day: 103, Value: 200}}
	if !slices.Equal(span.guild.Points, want) {
		t.Errorf("guild points = %+v", span.guild.Points)
	}
	if *span.guild.Median != 300 || span.guild.Dives != 5 {
		t.Errorf("guild median %v over %d", *span.guild.Median, span.guild.Dives)
	}
	days := []int{}
	for _, p := range span.global.Points {
		days = append(days, p.Day)
	}
	if !slices.Equal(days, []int{101, 103, 109}) || span.global.Points[0].Value != 367.5 {
		t.Errorf("global points = %+v", span.global.Points)
	}
	// 100 in 350-385, 50 under it and 50 over it: the middle 100 of 200
	if span.global.Dives != 200 || math.Abs(*span.global.Median-367.5) > 1e-9 {
		t.Errorf("global median %v over %d", *span.global.Median, span.global.Dives)
	}
	if span.window != "3-DAY" || len(span.guild.MA) != 2 || span.guild.MA[1].Value != 200 {
		t.Errorf("window %q, guild ma %+v", span.window, span.guild.MA)
	}
	// the guild's 300 against everyone's: 50 under 35, then 100 from 350 up
	if span.better == nil || math.Abs(*span.better-25) > 1e-9 {
		t.Errorf("better = %v", span.better)
	}

	if _, ok := buildSpan(entries, dist, 104, 109); ok {
		t.Error("a span with no guild games")
	}
	none := func(int) (krillionapi.Stats, bool) { return krillionapi.Stats{}, false }
	bare, _ := buildSpan(entries, none, 99, 109)
	if bare.global.Median != nil || bare.better != nil || len(bare.global.Points) != 0 {
		t.Errorf("no histograms: median %v, better %v", bare.global.Median, bare.better)
	}
}

// anything the card shows changes its name; nothing else does
func TestMedianPath(t *testing.T) {
	bcp := &types.BotCommandPackage{DataDir: "/data"}
	entries := []scores.Entry{entry("a", 100, 300), entry("b", 101, 400)}
	none := func(int) (krillionapi.Stats, bool) { return krillionapi.Stats{}, false }
	span, _ := buildSpan(entries, none, 1, 101)
	path := medianPath(bcp, "1", leaderboard.MONTH, span)
	dir, name := filepath.Split(path)
	if dir != "/data/dailygen/1/server/" || !strings.HasPrefix(name, "topg_month_100-101_") || !strings.HasSuffix(name, ".webp") {
		t.Errorf("medianPath = %q", path)
	}
	again, _ := buildSpan(entries, none, 1, 101)
	if medianPath(bcp, "1", leaderboard.MONTH, again) != path {
		t.Error("the same span named twice")
	}
	dropped, _ := buildSpan(append(entries, entry("c", 101, 100)), none, 1, 101)
	if medianPath(bcp, "1", leaderboard.MONTH, dropped) == path {
		t.Error("a new score kept the old name")
	}
}

func TestMedianPeriodsSkipAWeek(t *testing.T) {
	if slices.Contains(MEDIAN_PERIODS, leaderboard.WEEK) || !slices.Contains(MEDIAN_PERIODS, leaderboard.MONTH) {
		t.Errorf("MEDIAN_PERIODS = %v", MEDIAN_PERIODS)
	}
}

// a drawn card on disk for the span, plus another period's
func medianCard(t *testing.T, bcp *types.BotCommandPackage, span medianSpan) string {
	t.Helper()
	path := medianPath(bcp, "1", leaderboard.MONTH, span)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path, medianPath(bcp, "1", leaderboard.ALLTIME, span)} {
		if err := os.WriteFile(p, []byte("card"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// a card already on disk is reused: nothing is drawn
func TestCachedMedianReuses(t *testing.T) {
	bcp := &types.BotCommandPackage{DataDir: t.TempDir()}
	none := func(int) (krillionapi.Stats, bool) { return krillionapi.Stats{}, false }
	span, _ := buildSpan([]scores.Entry{entry("a", 100, 300)}, none, 1, 100)
	path := medianCard(t, bcp, span)
	err := cachedMedian(nil, path, "1", "Krill Club", leaderboard.MONTH, span, func() (string, error) {
		t.Error("drew a card already on disk")
		return "", nil
	})
	if err != nil {
		t.Errorf("cachedMedian: %v", err)
	}
}

// a card missing from disk while the guild's draw is underway isn't drawn
func TestCachedMedianTurnsAwayADuplicate(t *testing.T) {
	bcp := &types.BotCommandPackage{DataDir: t.TempDir()}
	none := func(int) (krillionapi.Stats, bool) { return krillionapi.Stats{}, false }
	span, _ := buildSpan([]scores.Entry{entry("a", 100, 300)}, none, 1, 100)
	_, done, err := claimDraw("1", SERVER)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	err = cachedMedian(nil, medianPath(bcp, "1", leaderboard.MONTH, span), "1", "Krill Club", leaderboard.MONTH, span, func() (string, error) {
		t.Error("drew while another draw was underway")
		return "", nil
	})
	var drawing *drawingError
	if !errors.As(err, &drawing) {
		t.Errorf("cachedMedian = %v, want a *drawingError", err)
	}
}

// the guild's draw is its own claim: players' /topu draws don't block it
func TestClearMedians(t *testing.T) {
	bcp := &types.BotCommandPackage{DataDir: t.TempDir()}
	none := func(int) (krillionapi.Stats, bool) { return krillionapi.Stats{}, false }
	span, _ := buildSpan([]scores.Entry{entry("a", 100, 300)}, none, 1, 100)
	medianCard(t, bcp, span)

	_, player, err := claimDraw("1", "2")
	if err != nil {
		t.Fatal(err)
	}
	defer player()
	_, done, err := claimDraw("1", SERVER)
	if err != nil {
		t.Fatalf("a player's draw blocked the server's: %v", err)
	}
	if n, busy, err := clearMedians(bcp, "1"); err != nil || !busy || n != 0 {
		t.Fatalf("clear mid-draw = %d %v %v", n, busy, err)
	}
	done()
	if n, busy, err := clearMedians(bcp, "1"); err != nil || busy || n != 2 {
		t.Fatalf("clear = %d %v %v", n, busy, err)
	}
	if _, err := os.Stat(medianDir(bcp, "1")); !os.IsNotExist(err) {
		t.Errorf("cards still there: %v", err)
	}
}
