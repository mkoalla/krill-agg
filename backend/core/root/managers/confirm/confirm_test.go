package confirm

import (
	"testing"
	"time"
)

func TestFlowDefaults(t *testing.T) {
	if got := (Flow{}).timeout(); got != DEFAULT_TIMEOUT || DEFAULT_TIMEOUT != 30*time.Second {
		t.Errorf("unset timeout = %v", got)
	}
	if got := (Flow{Timeout: 5 * time.Second}).timeout(); got != 5*time.Second {
		t.Errorf("set timeout = %v", got)
	}
	if (Flow{}).steps() != 1 || (Flow{Steps: 3}).steps() != 3 {
		t.Error("steps: unset should be one click")
	}
}

func TestQuestion(t *testing.T) {
	q := &pending{p: Prompt{Flow: Flow{Steps: 2}, Question: func(step int) string { return "Sure?" }}, step: 1, until: time.Unix(1000, 0)}
	if got := q.question(); got != "Sure? (1/2)\n-# expires <t:1000:R>" {
		t.Errorf("question = %q", got)
	}
}

func TestParseID(t *testing.T) {
	q := &pending{owner: "42", id: 7}
	runID, owner, id, action, ok := parseID(customID(q, yes))
	if !ok || owner != "42" || id != 7 || action != yes || runID == "" {
		t.Errorf("parseID = %q %q %d %q %v", runID, owner, id, action, ok)
	}
	for _, bad := range []string{PREFIX, PREFIX + "a:b:c", PREFIX + "a:b:x:yes"} {
		if _, _, _, _, ok := parseID(bad); ok {
			t.Errorf("parseID(%q) accepted", bad)
		}
	}
	if len(customID(&pending{owner: "123456789012345678901", id: 1 << 62}, yes)) > 100 {
		t.Error("custom id over discord's 100 characters")
	}
}

// each step gets the full timeout from when it starts
func TestRestart(t *testing.T) {
	q := &pending{p: Prompt{Flow: Flow{Steps: 2, Timeout: time.Minute}}}
	now := time.Unix(5000, 0)
	q.restart(now)
	if !q.until.Equal(now.Add(time.Minute)) {
		t.Errorf("until = %v", q.until)
	}
	later := now.Add(50 * time.Second)
	q.restart(later)
	if !q.until.Equal(later.Add(time.Minute)) {
		t.Errorf("until after a click = %v", q.until)
	}
}
