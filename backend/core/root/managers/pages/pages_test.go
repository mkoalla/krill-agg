package pages

import (
	"krill-agg/root/info/run"
	"testing"
)

func TestCustomIDRoundTrip(t *testing.T) {
	cid := customID("80351110224678912", 7, next)
	if len(cid) > 100 {
		t.Fatalf("custom id is %d chars; discord caps them at 100", len(cid))
	}
	id, ok := parseID(cid)
	if !ok || id != (clickID{run: run.ID, owner: "80351110224678912", epoch: 7, dir: next}) {
		t.Fatalf("parseID(%q) = %+v %v", cid, id, ok)
	}
	for _, bad := range []string{"pages:x", "pages:a:b:c:sideways", "pages:a:b:notanumber:next"} {
		if _, ok := parseID(bad); ok {
			t.Errorf("parseID(%q) accepted", bad)
		}
	}
}

func TestTurnWraps(t *testing.T) {
	if got := turn(0, 5, prev); got != 4 {
		t.Errorf("prev from first = %d, want 4", got)
	}
	if got := turn(4, 5, next); got != 0 {
		t.Errorf("next from last = %d, want 0", got)
	}
	if got := turn(2, 5, next); got != 3 {
		t.Errorf("next = %d, want 3", got)
	}
}

func TestLiveOnlyNewestOwnBook(t *testing.T) {
	man := New()
	first := &book{epoch: man.nextEpoch("1")}
	man.books["1"] = first
	newer := &book{epoch: man.nextEpoch("1")}
	man.books["1"] = newer

	if _, ok := man.live(clickID{run: run.ID, owner: "1", epoch: first.epoch}, "1"); ok {
		t.Error("a superseded book still flips")
	}
	if b, ok := man.live(clickID{run: run.ID, owner: "1", epoch: newer.epoch}, "1"); !ok || b != newer {
		t.Error("the newest book should flip")
	}
	if _, ok := man.live(clickID{run: run.ID, owner: "1", epoch: newer.epoch}, "2"); ok {
		t.Error("someone else flipped the owner's book")
	}
	if _, ok := man.live(clickID{run: "old-run", owner: "1", epoch: newer.epoch}, "1"); ok {
		t.Error("a book from a previous run still flips")
	}
}

func TestButtons(t *testing.T) {
	if got := buttons("1", 1, 1); len(got) != 0 {
		t.Errorf("single page got buttons: %v", got)
	}
	if got := buttons("1", 1, 5); len(got) != 1 {
		t.Errorf("want one row of << >>, got %v", got)
	}
	e := Embed("t", "", "body", Info{Cur: 2, Max: 5})
	if e.Footer.Text != "Page 2 / 5" || e.Thumbnail != nil {
		t.Errorf("Embed() = %+v", e)
	}
}
