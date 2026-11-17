package krills

import (
	"errors"
	"krill-agg/root/info/krillionapi"
	"krill-agg/root/krillion"
	"path/filepath"
	"testing"
)

func quip(s string) *string { return &s }

func reveal(day int) krillionapi.Reveal {
	return krillionapi.Reveal{Date: krillion.DateOf(day), Prompts: []krillionapi.RevealPrompt{
		{Text: "q1", Answers: []krillionapi.RevealAnswer{
			{Answer: "Pruno", Score: 100, Quip: quip(" prison wine ")},
			{Answer: "Oreo", Score: 30},
			{Answer: "Gizmo", Score: 100}, // no quip key at all
			{Answer: "Rare one", Score: 60, Quip: quip("not a krillion")},
		}},
		{Text: "q2", Answers: []krillionapi.RevealAnswer{{Answer: "Elephant", Score: 30}}},
		{Text: "q3", Answers: []krillionapi.RevealAnswer{{Answer: "Blank", Score: 100, Quip: quip("  ")}}},
	}}
}

func TestPick(t *testing.T) {
	got := Pick(reveal(5))
	if len(got) != 3 || len(got[0]) != 2 || len(got[1]) != 0 || len(got[2]) != 1 {
		t.Fatalf("Pick() = %+v", got)
	}
	if got[0][0] != (Krill{"Pruno", "prison wine"}) || got[0][1] != (Krill{Answer: "Gizmo"}) || got[2][0].Quip != "" {
		t.Errorf("Pick() = %+v", got)
	}
}

func TestFetchOnlyToday(t *testing.T) {
	path := filepath.Join(t.TempDir(), "krills.db")
	man, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	man.today = func() int { return 125 }
	man.fetch = func(date string) (krillionapi.Reveal, error) { calls++; return reveal(125), nil }

	if _, err := man.Fetch(124); !errors.Is(err, ErrNotToday) || calls != 0 {
		t.Fatalf("Fetch(yesterday) = %v after %d calls", err, calls)
	}
	if qs, err := man.Fetch(125); err != nil || len(qs) != 3 || calls != 1 {
		t.Fatalf("Fetch(today) = %v %v", qs, err)
	}
	if _, err := man.Fetch(125); err != nil || calls != 1 {
		t.Fatalf("second Fetch(today) called krillion again (%d)", calls)
	}
	man.Close() //nolint

	// saved days come back after a restart, even once they're past
	man, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer man.Close() //nolint
	man.today = func() int { return 126 }
	if qs, ok := man.Get(125); !ok || qs[0][0].Answer != "Pruno" {
		t.Fatalf("Get(125) after reopen = %+v %v", qs, ok)
	}
}
