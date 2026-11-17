package drop

import (
	"testing"
	"time"
)

func TestDropFlow(t *testing.T) {
	if DROP.Steps != 2 || DROP.Timeout != 30*time.Second {
		t.Errorf("DROP = %+v", DROP)
	}
}

func TestQuestion(t *testing.T) {
	q := question("ALL your scores", "12 scores")
	if got := q(1); got != "Are you sure you want to drop ALL your scores?\n- 12 scores" {
		t.Errorf("step 1 = %q", got)
	}
	if got := q(2); got != "Are you REALLY sure you want to drop ALL your scores?\n- 12 scores" {
		t.Errorf("step 2 = %q", got)
	}
}

func TestWhose(t *testing.T) {
	me, them := whose{user: "1", self: true}, whose{user: "2"}
	if me.their() != "your" || me.doesnt() != "You don't" {
		t.Errorf("self = %q %q", me.their(), me.doesnt())
	}
	if them.their() != "<@2>'s" || them.doesnt() != "<@2> doesn't" {
		t.Errorf("other = %q %q", them.their(), them.doesnt())
	}
}
