package drop

import (
	"strings"
	"testing"
	"time"
)

func TestFlowTimeout(t *testing.T) {
	if got := (Flow{Steps: 2}).timeout(); got != DEFAULT_TIMEOUT {
		t.Errorf("unset timeout = %v, want %v", got, DEFAULT_TIMEOUT)
	}
	if got := (Flow{Steps: 2, Timeout: 5 * time.Second}).timeout(); got != 5*time.Second {
		t.Errorf("set timeout = %v", got)
	}
	if DROP.timeout() != 30*time.Second || DROP.Steps != 2 {
		t.Errorf("DROP = %+v", DROP)
	}
}

func TestQuestion(t *testing.T) {
	at := time.Unix(1000, 0)
	p := &prompt{flow: DROP, step: 1, what: "ALL your scores", shown: "12 scores"}
	if got := question(p, at); got != "Are you sure you want to drop ALL your scores? (1/2)\n- 12 scores\n-# expires <t:1000:R>" {
		t.Errorf("step 1 = %q", got)
	}
	p.step = 2
	if got := question(p, at); !strings.HasPrefix(got, "Are you REALLY sure you want to drop ALL your scores? (2/2)") {
		t.Errorf("step 2 = %q", got)
	}
}

func TestParseID(t *testing.T) {
	p := &prompt{owner: "42", id: 7}
	runID, owner, id, action, ok := parseID(customID(p, confirm))
	if !ok || owner != "42" || id != 7 || action != confirm || runID == "" {
		t.Errorf("parseID = %q %q %d %q %v", runID, owner, id, action, ok)
	}
	for _, bad := range []string{"drop:", "drop:a:b:c", "drop:a:b:x:confirm"} {
		if _, _, _, _, ok := parseID(bad); ok {
			t.Errorf("parseID(%q) accepted", bad)
		}
	}
	if len(customID(&prompt{owner: "123456789012345678901", id: 1 << 62}, confirm)) > 100 {
		t.Error("custom id over discord's 100 characters")
	}
}

// each step gets the full timeout from when it starts
func TestRestart(t *testing.T) {
	p := &prompt{flow: Flow{Steps: 2, Timeout: time.Minute}}
	now := time.Unix(5000, 0)
	p.restart(now)
	if !p.until.Equal(now.Add(time.Minute)) {
		t.Errorf("until = %v", p.until)
	}
	later := now.Add(50 * time.Second)
	p.restart(later)
	if !p.until.Equal(later.Add(time.Minute)) {
		t.Errorf("until after a click = %v", p.until)
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
