package quantile

import (
	"krill-agg/root/info/krillionapi"
	"krill-agg/root/managers/scores"
	"math"
	"strings"
	"testing"
)

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestProbit(t *testing.T) {
	for p, z := range map[float64]float64{0.5: 0, 0.9: 1.2815516, 0.95: 1.6448536, 0.975: 1.9599640, 0.1: -1.2815516} {
		if got := probit(p); !near(got, z, 1e-6) {
			t.Errorf("probit(%v) = %v, want %v", p, got, z)
		}
	}
}

// running totals for these scores
func moments(scores_ ...int) scores.Moments {
	m := scores.Moments{}
	for _, s := range scores_ {
		m.N++
		m.Sum += int64(s)
		m.SumSq += int64(s) * int64(s)
	}
	return m
}

func TestGuildEstimate(t *testing.T) {
	// mean 400, sd 100 (sample): the top 10% starts 1.28 sds up
	m := moments(300, 400, 500, 300, 400, 500)
	sd := math.Sqrt(40000.0 / 5)
	e, ok := guildEstimate(m, 0.9, 0.9)
	if !ok || !near(e.score, 400+1.2815516*sd, 1e-4) {
		t.Errorf("top 10%% = %+v %v", e, ok)
	}
	// the interval: z_ci · sd · sqrt(1/n + z²/2(n-1))
	if want := 1.6448536 * sd * math.Sqrt(1.0/6+1.2815516*1.2815516/10); !near(e.err, want, 1e-4) {
		t.Errorf("err = %v, want %v", e.err, want)
	}
	// the median has no spread term, and a wider interval is wider
	mid, _ := guildEstimate(m, 0.5, 0.9)
	wide, _ := guildEstimate(m, 0.5, 0.99)
	if !near(mid.score, 400, 1e-9) || !near(mid.err, 1.6448536*sd/math.Sqrt(6), 1e-4) || wide.err <= mid.err {
		t.Errorf("median = %+v, wider = %+v", mid, wide)
	}
	// more games, a tighter interval
	many := moments(300, 400, 500, 300, 400, 500, 300, 400, 500, 300, 400, 500)
	if tight, _ := guildEstimate(many, 0.9, 0.9); tight.err >= e.err {
		t.Errorf("12 games %v, not tighter than 6's %v", tight.err, e.err)
	}
	// stays on the board
	if top, _ := guildEstimate(moments(690, 700, 650), 0.9999, 0.9); top.score != MAX_SCORE {
		t.Errorf("top 0.01%% = %v, past %v", top.score, MAX_SCORE)
	}
	if bottom, _ := guildEstimate(moments(0, 10, 40), 0.0001, 0.9); bottom.score != 0 {
		t.Errorf("top 99.99%% = %v, under 0", bottom.score)
	}
	if _, ok := guildEstimate(moments(400), 0.9, 0.9); ok {
		t.Error("one game has a spread")
	}
}

func TestGlobalEstimate(t *testing.T) {
	dist := make([]float64, 20)
	dist[9], dist[10] = 500, 500 // 315-385, centered on 350
	e, ok := globalEstimate(krillionapi.Stats{Sample: 1000, Dist: dist}, 0.75, 0.9)
	if !ok {
		t.Fatal("no estimate")
	}
	if want, _ := krillionapi.Quantile(dist, 0.75); e.score != want || !near(e.score, 367.5, 1e-9) {
		t.Errorf("score = %v, want the in-bucket quantile 367.5", e.score)
	}
	// sampling (about 1 from 1000 games 17.5 apart) plus placement: halfway
	// into bucket 10, whose edges average 500 and 250 players
	place := placementError(dist, e.score)
	if !near(place, 250*0.25/2/500*35, 1e-9) || e.err <= place || e.err > place+2 {
		t.Errorf("err = %v, placement %v", e.err, place)
	}
	if _, ok := globalEstimate(krillionapi.Stats{Dist: make([]float64, 20)}, 0.5, 0.9); ok {
		t.Error("an estimate with no histogram")
	}
}

func TestEmbed(t *testing.T) {
	e := embed("Trick", &estimate{412.4, 8.6}, &estimate{398.2, 0.3}, &estimate{421.7, 3.2}, 97.5, 90)
	if e.Title != "Krillion Scores to Beat 97.5%" {
		t.Errorf("title = %q", e.Title)
	}
	want := "**Global (Today)**: 422 ± 3\n**Global**: 398 ± 0\n**Trick**: 412 ± 9\n-# 90% confidence interval"
	if e.Description != want {
		t.Errorf("description = %q, want %q", e.Description, want)
	}
	none := embed("Trick", nil, nil, nil, 10, 99.5)
	if !strings.Contains(none.Description, "not enough games") || !strings.Contains(none.Description, "no histogram for today") || !strings.Contains(none.Description, "-# 99.5% confidence") {
		t.Errorf("empty = %q", none.Description)
	}
}

func TestPlacementError(t *testing.T) {
	flat := []float64{100, 100, 100, 100}
	if got := placementError(flat, 52.5); got != 0 {
		t.Errorf("even buckets misplace by %v", got)
	}
	falling := []float64{400, 200, 100, 50}
	// bucket edges are exact; mid-bucket is the worst
	edge, quarter, mid := placementError(falling, 35), placementError(falling, 43.75), placementError(falling, 52.5)
	if edge != 0 || quarter <= 0 || mid <= quarter {
		t.Errorf("edge %v, quarter %v, mid %v", edge, quarter, mid)
	}
	// edges average 300 and 150: 150·¼/2 players over 200 per bucket
	if !near(mid, 150*0.25/2/200*35, 1e-9) {
		t.Errorf("mid = %v", mid)
	}
	if got := placementError([]float64{0, 100}, 10); got != 0 {
		t.Errorf("an empty bucket misplaces by %v", got)
	}
}
