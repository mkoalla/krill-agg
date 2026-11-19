package help

import (
	"krill-agg/root/managers/pages"
	"os"
	"strings"
	"testing"
)

func TestCommandHelpParses(t *testing.T) {
	raw, err := os.ReadFile("../../../.command_help") // the module root, beside main.go
	if err != nil {
		t.Fatal(err)
	}
	sections, err := parse(string(raw))
	if err == nil {
		sections, err = arrange(sections)
	}
	if err != nil {
		t.Fatalf(".command_help: %v", err)
	}
	seen := map[string]bool{}
	count := 0
	for _, sec := range sections {
		for _, e := range sec.entries {
			if seen[e.usage] {
				t.Errorf("%s listed twice", e.usage)
			}
			seen[e.usage] = true
			count++
		}
	}
	if count < 10 {
		t.Fatalf("only %d entries", count)
	}
	book := helpBook(sections)
	if len(book) != len(layout)+1 {
		t.Fatalf("%d pages, want %d", len(book), len(layout)+1)
	}
	for j, page := range book {
		embed, err := page(pages.Info{Cur: j + 1, Max: len(book)})
		if err != nil {
			t.Fatal(err)
		}
		if n := len(embed.Description); n > MAX_DESCRIPTION {
			t.Errorf("/help page %q is %d characters, over discord's %d", embed.Title, n, MAX_DESCRIPTION)
		}
	}
}

func TestParse(t *testing.T) {
	got, err := parse("!One\n/a || [x]\nDoes a.\n\n/b\nDoes b.\n\n!Two\n/c\nDoes c.\n")
	if err != nil || len(got) != 2 || got[0].name != "One" || len(got[0].entries) != 2 ||
		got[0].entries[0] != (entry{"/a || [x]", "Does a."}) || got[1].name != "Two" || got[1].entries[0].usage != "/c" {
		t.Fatalf("parse() = %+v %v", got, err)
	}
	for _, bad := range []string{
		"/a\nDoes a.\n",                    // no section
		"!One\n/a\n",                       // no description
		"!One\na\nDoes a.\n",               // not a command
		"!One\n/a\nDoes a.\n/b\nDoes b.\n", // no blank line
		"!One\n/a\n\n",                     // empty description
		"!One\n!Two\n/a\nDoes a.\n",        // empty section
		"!One\n/a\nDoes a.\n\n!Two\n",      // empty last section
		"!\n/a\nDoes a.\n",                 // unnamed section
	} {
		if _, err := parse(bad); err == nil {
			t.Errorf("parse(%q) accepted", bad)
		}
	}
	if body := sectionBody("blurb", got[0].entries); !strings.Contains(body, "**`/a || [x]`**\nDoes a.") || !strings.HasPrefix(body, "*blurb*") {
		t.Errorf("body = %q", body)
	}
}

func TestArrange(t *testing.T) {
	all := make([]section, len(layout))
	for j, l := range layout {
		all[len(layout)-1-j] = section{name: l.name, entries: []entry{{"/x", "X."}}} // reversed
	}
	got, err := arrange(all)
	if err != nil {
		t.Fatal(err)
	}
	for j, l := range layout {
		if got[j].name != l.name {
			t.Errorf("page %d = %q, want %q", j, got[j].name, l.name)
		}
	}
	if _, err := arrange(all[1:]); err == nil {
		t.Error("missing section accepted")
	}
	if _, err := arrange(append(all, section{name: "Extra"})); err == nil {
		t.Error("unknown section accepted")
	}
	if _, err := arrange(append(all, all[0])); err == nil {
		t.Error("duplicate section accepted")
	}
}
