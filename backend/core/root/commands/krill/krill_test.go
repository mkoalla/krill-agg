package krill

import (
	"krill-agg/root/managers/pages"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestAnswersBody(t *testing.T) {
	day := [][]Krill{{{Answer: "Pruno", Quip: "prison wine"}, {Answer: "Gizmo"}}, {}}
	got := answersBody([]string{"Name a food ending in O", "Name a fish"}, day)
	want := "1\\. ||Name a food ending in O\n• Pruno\n• Gizmo||\n\n2\\. ||Name a fish||\n• N/A"
	if got != want {
		t.Errorf("answersBody() = %q\nwant %q", got, want)
	}
	// questions not saved: the blocks are just the answers
	if got := answersBody(nil, day); got != "1\\. ||• Pruno\n• Gizmo||\n\n2\\. • N/A" {
		t.Errorf("answersBody(no questions) = %q", got)
	}
	if got := answersBody(nil, [][]Krill{{}, {}}); got != NO_KRILLIONS {
		t.Errorf("no krillions = %q", got)
	}
}

func TestQuipsBody(t *testing.T) {
	day := [][]Krill{{{Answer: "Pruno", Quip: "prison wine"}, {Answer: "Gizmo"}}, {{Answer: "Rhapsody", Quip: "a skin"}}}
	if got := quipsBody(day); got != "||Pruno\n• prison wine\n\nRhapsody\n• a skin||" {
		t.Errorf("quipsBody() = %q", got)
	}
	if got := quipsBody([][]Krill{{{Answer: "Gizmo"}}}); got != NO_QUIPS {
		t.Errorf("no quips = %q", got)
	}
}

func TestClip(t *testing.T) {
	for _, long := range []string{strings.Repeat("x", 5000), strings.Repeat("🌟", 5000)} {
		got := clip(long)
		if n := utf8.RuneCountInString(got); n != MAX_DESCRIPTION || !utf8.ValidString(got) {
			t.Errorf("clip left %d characters (valid utf-8: %v)", n, utf8.ValidString(got))
		}
	}
	if clip("short") != "short" {
		t.Error("clip changed a short string")
	}
}

func TestBook(t *testing.T) {
	b := book(124, []string{"q"}, [][]Krill{{{Answer: "a", Quip: "ha"}}})
	if len(b) != 2 {
		t.Fatalf("%d pages", len(b))
	}
	answers, _ := b[0](pages.Info{Cur: 1, Max: 2})
	quips, _ := b[1](pages.Info{Cur: 2, Max: 2})
	if !strings.Contains(answers.Title, "#124") || !strings.Contains(answers.Description, "||q\n• a||") {
		t.Errorf("answers = %q / %q", answers.Title, answers.Description)
	}
	if !strings.Contains(quips.Title, "Quips") || quips.Description != "||a\n• ha||" || quips.Footer.Text != "Page 2 / 2" {
		t.Errorf("quips = %q / %q / %q", quips.Title, quips.Description, quips.Footer.Text)
	}
}
