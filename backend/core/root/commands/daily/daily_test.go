package daily

import (
	"krill-agg/root/info/krillionapi"
	"krill-agg/root/krillion"
	"krill-agg/root/managers/scores"
	"testing"
)

func TestCommas(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 3245071: "3,245,071", -12345: "-12,345"} {
		if got := commas(n); got != want {
			t.Errorf("commas(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestPercentileLine(t *testing.T) {
	for p, want := range map[float64]string{5.9: "`~05%` - <@1>", 53.69: "`~53%` - <@1>", 99.99: "`~99%` - <@1>", 100: "`~100%` - <@1>"} {
		if got := percentileLine("1", p); got != want {
			t.Errorf("percentileLine(%v) = %q, want %q", p, got, want)
		}
	}
}

func TestHeading(t *testing.T) {
	if got := Heading(124); got != "# Krillion Daily Results #124 🦐" {
		t.Errorf("Heading() = %q", got)
	}
}

func TestWinners(t *testing.T) {
	entry := func(uid string, emojis string) scores.Entry {
		p, err := krillion.Rebuild(124, emojis)
		if err != nil {
			t.Fatal(err)
		}
		return scores.Entry{UserID: uid, Score: p}
	}
	top, tie, low := entry("1", "🦑🦑🦑🐟🦑🦑🏮"), entry("2", "🦑🦑🦑🐟🦑🦑🏮"), entry("3", "⬛⬛⬛⬛⬛⬛⬛")
	if got := Winners(nil); got != "" {
		t.Errorf("no players: %q", got)
	}
	if got := Winners([]scores.Entry{top, low}); got != "Today's Winner: <@1>" {
		t.Errorf("one winner: %q", got)
	}
	if got := Winners([]scores.Entry{top, tie, low}); got != "Today's Winners:\n- <@1>\n- <@2>" {
		t.Errorf("tie: %q", got)
	}
	snap := &Snapshot{Day: 124, Entries: []scores.Entry{top, low}}
	if got := Content(snap, true); got != "# Krillion Daily Results #124 🦐\nToday's Winner: <@1>" {
		t.Errorf("Content() = %q", got)
	}
	if got := Content(snap, false); got != "# Krillion Daily Results #124 🦐" {
		t.Errorf("Content(no winners) = %q", got)
	}
	if got := Content(&Snapshot{Day: 124}, true); got != "# Krillion Daily Results #124 🦐" {
		t.Errorf("empty Content() = %q", got)
	}
}

func TestCombined(t *testing.T) {
	dist := krillionapi.Stats{Sample: 3245071, Dist: []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3245071, 0, 0, 0, 0, 0, 0, 0, 0, 0}}
	entries := []scores.Entry{{UserID: "1", Score: krillion.PlayerScore{Day: 124, Score: 415, Choices: []string{"🦑", "🦑", "🦑", "🐟", "🦑", "🦑", "🏮"}}}}
	want := "`🦑🦑🦑🐟🦑🦑🏮` - `415` - <@1>\n### Percentiles\nPlayers today: `3,245,071`\n\n`~100%` - <@1>"
	got := Combined(124, entries, dist)
	if got.Title != "Scores" || got.Description != want {
		t.Errorf("Combined() = %q / %q\nwant %q", got.Title, got.Description, want)
	}

	empty := Combined(124, nil, dist)
	if empty.Description != "No players yet!\n### Percentiles\nPlayers today: `3,245,071`" {
		t.Errorf("empty Combined() = %q", empty.Description)
	}
}
