package questions

import (
	"errors"
	"krill-agg/root/info/krillionapi"
	"path/filepath"
	"slices"
	"testing"
)

func TestFetchSavesToday(t *testing.T) {
	path := filepath.Join(t.TempDir(), "questions.db")
	man, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	man.fetch = func() (krillionapi.Puzzle, error) {
		return krillionapi.Puzzle{DayNumber: 124, Prompts: []krillionapi.Prompt{{Text: "Name a fish"}, {Text: "Name a squid"}}}, nil
	}
	if _, ok := man.Get(124); ok {
		t.Fatal("questions before any fetch")
	}
	if _, err := man.Fetch(123); !errors.Is(err, ErrNotToday) {
		t.Fatalf("Fetch(yesterday) = %v, want ErrNotToday", err)
	}
	qs, err := man.Fetch(124)
	if err != nil || !slices.Equal(qs, []string{"Name a fish", "Name a squid"}) {
		t.Fatalf("Fetch(124) = %v %v", qs, err)
	}
	man.Close() //nolint

	man, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer man.Close() //nolint
	if qs, ok := man.Get(124); !ok || len(qs) != 2 {
		t.Fatalf("after reopen Get(124) = %v %v", qs, ok)
	}
}
