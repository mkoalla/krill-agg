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
