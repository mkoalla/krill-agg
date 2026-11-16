package krillion

import (
	"errors"
	"strings"
	"testing"
)

// copied byte-for-byte from krillion.example.txt, one block per valid result
func TestFromMessage_Examples(t *testing.T) {
	cases := []struct {
		name  string
		input string
		day   int
		score int
	}{
		{"124", "Krillion #124 🦐\n415\n\n🦑🦑🦑🐟🦑🦑🏮", 124, 415},
		{"123a", "Krillion #123 🦐\n495\n\n🦑🏮🏮🏮🏮🫧🏮", 123, 495},
		{"123b", "Krillion #123 🦐\n310\n\n🏮🫧🫧🏮🐟🦑🐟", 123, 310},
		{"122", "Krillion #122 🦐\n265\n\n🤡🦑🐟🐟🦑🦑🫧", 122, 265},
		{"120", "Krillion #120 🦐\n210\n\n🐟🐟🐟⬛🐟🦑🐟", 120, 210},
		{"119", "Krillion #119 🦐\n370\n\n🐟🏮🏮⬛🏮⬛🏮", 119, 370},
		{"116", "Krillion #116 🦐\n370\n\n🐟🫧⬛🏮🌟🦑🏮", 116, 370},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := FromMessage(c.input)
			if err != nil {
				t.Fatalf("FromMessage() error = %v", err)
			}
			if p.Day != c.day {
				t.Errorf("Day = %d, want %d", p.Day, c.day)
			}
			if p.Score != c.score {
				t.Errorf("Score = %d, want %d", p.Score, c.score)
			}
			if len(p.Choices) != NumChoices {
				t.Errorf("len(Choices) = %d, want %d", len(p.Choices), NumChoices)
			}
		})
	}
}

// copied byte-for-byte from krillion.example.bad.txt (minus the trailing notes,
// which aren't part of the message); must fail with both reasons
func TestFromMessage_Bad(t *testing.T) {
	input := "Krillion #118 🐏 \n488\n\n🦑🦑🦑🦑🦑🦑🏮✡️"
	_, err := FromMessage(input)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "unknown score type: ✡️") {
		t.Errorf("error missing unknown-type reason: %q", msg)
	}
	if !strings.Contains(msg, "score mismatched to emojis") {
		t.Errorf("error missing sum mismatch reason: %q", msg)
	}
}

// the parser should tolerate chat noise around a well-formed result
func TestFromMessage_Permissive(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{
			"leading chat text",
			"hey check this out: Krillion #5\n420\n\n🦑🦑🦑🦑🦑🦑🦑",
		},
		{
			"trailing chat text after a space",
			"Krillion #5\n420\n\n🦑🦑🦑🦑🦑🦑🦑 nice game!",
		},
		{
			"lowercase header",
			"krillion #9\n0\n\n⬛⬛⬛⬛⬛⬛⬛",
		},
		{
			"no space before the #",
			"Krillion#8\n420\n\n🦑🦑🦑🦑🦑🦑🦑",
		},
		{
			"extra blank lines before the emoji chain",
			"Krillion #11\n0\n\n\n\n⬛⬛⬛⬛⬛⬛⬛",
		},
		{
			"variation-selected emoji",
			"Krillion #12\n0\n\n" + strings.Repeat("⬛️", NumChoices),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := FromMessage(c.input); err != nil {
				t.Errorf("FromMessage(%q) error = %v", c.input, err)
			}
		})
	}
}

// the emoji chain stops at the first non-scoring cluster. plain text glued on
// just ends it; an unknown emoji is remembered for validation
func TestParse_ChainStopsAtJunk(t *testing.T) {
	p, err := Parse("Krillion #5\n60\n\n🦑lol")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(p.Choices) != 1 || p.Choices[0] != "🦑" {
		t.Errorf("Choices = %v, want [🦑]", p.Choices)
	}
	if p.stoppedAt != "" {
		t.Errorf("stoppedAt = %q, want empty", p.stoppedAt)
	}

	p, err = Parse("Krillion #5\n60\n\n🦑✡️")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if p.stoppedAt != "✡️" {
		t.Errorf("stoppedAt = %q, want %q", p.stoppedAt, "✡️")
	}
}

func TestParse_Failures(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"missing day", "Krillion # oops\n100\n\n🦑🦑🦑🦑🦑🦑🦑"},
		{"missing score", "Krillion #5 🦐\nhello\n\n🦑🦑🦑🦑🦑🦑🦑"},
		{"missing emojis", "Krillion #5\n100\n\nhello world"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(c.input)
			if _, ok := errors.AsType[*ParseError](err); !ok {
				t.Fatalf("Parse() error = %v, want *ParseError", err)
			}
		})
	}
}

// ordinary chat with no krillion header must be silently ignorable
func TestFromMessage_NotKrillion(t *testing.T) {
	for _, input := range []string{"hello", "I love krill"} {
		_, err := FromMessage(input)
		if !errors.Is(err, ErrNotKrillion) {
			t.Errorf("FromMessage(%q) error = %v, want ErrNotKrillion", input, err)
		}
	}
}

func TestNextCluster(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		cluster string
		rest    string
	}{
		{"variation selector", "✡️x", "✡️", "x"},
		{"skin tone", "\U0001F44D\U0001F3FDx", "\U0001F44D\U0001F3FD", "x"},
		{"zwj family", "\U0001F468\u200d\U0001F469\u200d\U0001F467x", "\U0001F468\u200d\U0001F469\u200d\U0001F467", "x"},
		{"flag", "\U0001F1FA\U0001F1F8x", "\U0001F1FA\U0001F1F8", "x"},
		{"plain ascii", "abc", "a", "bc"},
		{"empty", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cluster, rest := nextCluster(c.input)
			if cluster != c.cluster || rest != c.rest {
				t.Errorf("nextCluster(%q) = (%q, %q), want (%q, %q)", c.input, cluster, rest, c.cluster, c.rest)
			}
		})
	}
}
