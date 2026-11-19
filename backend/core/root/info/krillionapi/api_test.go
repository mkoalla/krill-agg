package krillionapi

import (
	"math"
	"testing"
)

// a real snapshot (sample is always the bucket total) plus the betterThan
// krillion reported for each score
var snapshot = Stats{
	Sample: 3242887,
	Dist:   []float64{2155, 9872, 46531, 83029, 192709, 262968, 383812, 480162, 489957, 382995, 367690, 271100, 122620, 68850, 44903, 15027, 10123, 6290, 1844, 250},
}

func TestPercentile(t *testing.T) {
	// within a point of what the api said, except 400 (players cluster unevenly in that bucket)
	cases := map[float64]float64{0: 0, 175: 10, 245: 30, 250: 32, 265: 38, 280: 45, 300: 53, 350: 72, 455: 95, 525: 99, 630: 99.94, 665: 99.99}
	for score, want := range cases {
		if got := snapshot.Percentile(score); math.Abs(got-want) > 1 {
			t.Errorf("Percentile(%v) = %.2f, api said %.2f", score, got, want)
		}
	}
	if got := snapshot.Percentile(700); got != 100 {
		t.Errorf("Percentile(700) = %.3f, want 100", got)
	}
	if got := (Stats{}).Percentile(300); got != 0 {
		t.Errorf("empty stats = %v, want 0", got)
	}
}

// Quantile runs Percentile backwards
func TestQuantile(t *testing.T) {
	dist := []float64{10, 30, 60}
	s := Stats{Sample: 100, Dist: dist}
	for _, q := range []float64{0.05, 0.1, 0.25, 0.5, 0.9, 0.99} {
		score, ok := Quantile(dist, q)
		if !ok {
			t.Fatalf("Quantile(%v) not ok", q)
		}
		if back := s.Percentile(score); math.Abs(back-100*q) > 1e-9 {
			t.Errorf("Quantile(%v) = %v, which is percentile %v", q, score, back)
		}
	}
	if _, ok := Quantile(make([]float64, 20), 0.5); ok {
		t.Error("an empty histogram has a quantile")
	}
}

func TestFormatPercent(t *testing.T) {
	for p, want := range map[float64]string{95.6549: "95.65", 5.9: "5.90", 100: "100.00", 99.999: "99.99", 0: "0.00"} {
		if got := FormatPercent(p); got != want {
			t.Errorf("FormatPercent(%v) = %q, want %q", p, got, want)
		}
	}
}

func TestMisplaced(t *testing.T) {
	falling := []float64{400, 200, 100, 50}
	// exact at a bucket edge, with even buckets and in an empty bucket
	if m := Misplaced(falling, 35); m != 0 {
		t.Errorf("edge misplaces %v", m)
	}
	if m := Misplaced([]float64{100, 100, 100, 100}, 50); m != 0 {
		t.Errorf("even buckets misplace %v", m)
	}
	if m := Misplaced([]float64{100, 0, 100}, 50); m != 0 {
		t.Errorf("an empty bucket misplaces %v", m)
	}
	// mid-bucket 1: edges average 300 and 150, so 150·¼/2 players
	if m := Misplaced(falling, 52.5); math.Abs(m-150*0.25/2) > 1e-9 {
		t.Errorf("mid misplaces %v", m)
	}
}
