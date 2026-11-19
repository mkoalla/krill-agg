package scores

import (
	"path/filepath"
	"testing"
)

// what a fresh full read of the guild gives
func recount(t *testing.T, man *ScoreManager, guild string) Moments {
	t.Helper()
	entries, err := man.Guild(guild, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	m := Moments{}
	for _, e := range entries {
		m.add(e.Score.Score, 1)
	}
	return m
}

// every kind of write keeps the running totals equal to a full read
func TestMomentsFollowWrites(t *testing.T) {
	man := open(t, filepath.Join(t.TempDir(), "scores.db"))
	defer man.Close() //nolint
	check := func(step string) {
		t.Helper()
		got, err := man.Moments("1")
		if err != nil {
			t.Fatal(err)
		}
		if want := recount(t, man, "1"); got != want {
			t.Fatalf("after %s: running %+v, full read %+v", step, got, want)
		}
	}

	check("nothing") // and starts keeping them
	man.Ingest("1", "10", day123)
	check("ingest")
	man.Ingest("1", "10", alt123) // a conflict: nothing stored
	check("conflict")
	man.Ingest("2", "10", day124) // another guild
	check("another guild")
	r := man.Ingest("1", "20", day124)
	check("second player")
	alt := man.Ingest("3", "10", alt123).Score
	if err := man.Set("1", "10", alt); err != nil { // overwrites 495 with 310
		t.Fatal(err)
	}
	check("overwrite")
	if err := man.Set("1", "30", alt); err != nil { // a new entry
		t.Fatal(err)
	}
	check("set new")
	if _, err := man.Drop("1", "20", r.Score.Day); err != nil {
		t.Fatal(err)
	}
	check("drop")
	if _, err := man.Drop("1", "20", r.Score.Day); err != nil { // already gone
		t.Fatal(err)
	}
	check("drop again")
	if err := man.SetMany("1", []Entry{{UserID: "40", Score: alt}, {UserID: "10", Score: r.Score}}); err != nil {
		t.Fatal(err)
	}
	check("set many")
	if _, err := man.DropAll("1", "10"); err != nil {
		t.Fatal(err)
	}
	check("drop all")

	if m, _ := man.Moments("1"); m.N != 2 || m.Sum != 620 || m.SumSq != 2*310*310 {
		t.Errorf("final = %+v, want two 310s", m)
	}
}
