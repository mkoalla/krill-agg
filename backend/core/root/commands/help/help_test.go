package help

import (
	"os"
	"strings"
	"testing"
)

func TestCommandHelpParses(t *testing.T) {
	raw, err := os.ReadFile("../../../.command_help") // the module root, beside main.go
	if err != nil {
		t.Fatal(err)
	}
	entries, err := parse(string(raw))
	if err != nil {
		t.Fatalf(".command_help: %v", err)
	}
	if len(entries) < 10 {
		t.Fatalf("only %d entries", len(entries))
	}
	if n := len(helpEmbed(entries).Description); n > MAX_DESCRIPTION {
		t.Errorf("/help is %d characters, over discord's %d", n, MAX_DESCRIPTION)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if seen[e.usage] {
			t.Errorf("%s listed twice", e.usage)
		}
		seen[e.usage] = true
	}
}

func TestParse(t *testing.T) {
	got, err := parse("/a || [x]\nDoes a.\n\n/b\nDoes b.\n")
	if err != nil || len(got) != 2 || got[0] != (entry{"/a || [x]", "Does a."}) || got[1].usage != "/b" {
		t.Fatalf("parse() = %+v %v", got, err)
	}
	for _, bad := range []string{"/a\n", "a\nDoes a.\n", "/a\nDoes a.\n/b\nDoes b.\n", "/a\n\n"} {
		if _, err := parse(bad); err == nil {
			t.Errorf("parse(%q) accepted", bad)
		}
	}
	if !strings.Contains(helpEmbed(got).Description, "**`/a || [x]`**\nDoes a.") {
		t.Errorf("embed = %q", helpEmbed(got).Description)
	}
}
