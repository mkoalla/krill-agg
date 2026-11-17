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
