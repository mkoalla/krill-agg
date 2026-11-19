package distributions

import (
	"path/filepath"
	"testing"
)

func TestSaveAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "distributions.db")
	man, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := man.Save(60, Stats{Sample: 295142, BetterThan: 94, Dist: []float64{287, 2094}}); err != nil {
		t.Fatal(err)
	}
	if err := man.Save(1, Stats{}); err != nil { // krillion's "no data" answer
		t.Fatal(err)
	}
	man.Close() //nolint

	man, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer man.Close() //nolint
	if s, ok := man.Get(60); !ok || s.Sample != 295142 || len(s.Dist) != 2 || !HasDist(s) {
		t.Errorf("Get(60) = %+v %v", s, ok)
	}
	if s, ok := man.Get(1); !ok || HasDist(s) {
		t.Errorf("Get(1) = %+v %v, want saved with no dist", s, ok)
	}
	if _, ok := man.Get(2); ok {
		t.Error("unsaved day found")
	}
}

// the all-time total follows saves, replaced days included
func TestAllTime(t *testing.T) {
	man, err := New(filepath.Join(t.TempDir(), "distributions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer man.Close() //nolint
	if _, ok := man.AllTime(); ok {
		t.Error("a total with nothing saved")
	}
	man.Save(1, Stats{Sample: 10, Dist: []float64{4, 6}}) //nolint
	man.Save(2, Stats{})                                  //nolint // no histogram: left out
	if s, ok := man.AllTime(); !ok || s.Sample != 10 || s.Dist[0] != 4 || s.Dist[1] != 6 {
		t.Errorf("first = %+v %v", s, ok)
	}
	man.Save(3, Stats{Sample: 5, Dist: []float64{0, 2, 3}}) //nolint
	man.Save(1, Stats{Sample: 3, Dist: []float64{1, 2}})    //nolint // replaced: the old one comes out
	s, ok := man.AllTime()
	if !ok || s.Sample != 8 || len(s.Dist) != 3 || s.Dist[0] != 1 || s.Dist[1] != 4 || s.Dist[2] != 3 {
		t.Errorf("after more = %+v %v", s, ok)
	}
	s.Dist[0] = 99 // a copy: the running total is untouched
	if again, _ := man.AllTime(); again.Dist[0] != 1 {
		t.Error("AllTime handed out its own total")
	}
}
