package top

import (
	"krill-agg/root/managers/scores"
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
