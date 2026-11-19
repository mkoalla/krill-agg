package daily

import (
	"errors"
	"krill-agg/root/info/krillionapi"
	"krill-agg/root/krillion"
	"krill-agg/root/managers/scores"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestCommas(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 3245071: "3,245,071", -12345: "-12,345"} {
		if got := commas(n); got != want {
			t.Errorf("commas(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestPercentileLine(t *testing.T) {
	if got := percentileLine("1", "95.65"); got != "`95.65%` - <@1>" {
		t.Errorf("percentileLine() = %q", got)
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
	today := krillion.Today()
	dist := krillionapi.Stats{Sample: 3245071, Dist: []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3245071, 0, 0, 0, 0, 0, 0, 0, 0, 0}}
	entries := []scores.Entry{{UserID: "1", Score: krillion.PlayerScore{Day: today, Score: 415, Choices: []string{"🦑", "🦑", "🦑", "🐟", "🦑", "🦑", "🏮"}}}}
	want := "**Scores**\n`🦑🦑🦑🐟🦑🦑🏮` - `415` - <@1>\n\n**Percentiles**\nPlayers today: `3,245,071`\n\n`100.00%` - <@1>"
	got := Combined(today, entries, dist)
	if got.Title != "" || got.Description != want {
		t.Errorf("Combined() = %q / %q\nwant %q", got.Title, got.Description, want)
	}

	empty := Combined(today, nil, dist)
	if empty.Description != "**Scores**\nNo players yet!\n\n**Percentiles**\nPlayers today: `3,245,071`" {
		t.Errorf("empty Combined() = %q", empty.Description)
	}

	// a finished day counts "that day"; one krillion has no distribution for says so
	if past := Combined(today-1, nil, dist); past.Description != "**Scores**\nNo players yet!\n\n**Percentiles**\nPlayers that day: `3,245,071`" {
		t.Errorf("past Combined() = %q", past.Description)
	}
	if none := Combined(5, nil, krillionapi.Stats{}); none.Description != "**Scores**\nNo players yet!\n\n**Percentiles**\n"+NO_DIST {
		t.Errorf("no-dist Combined() = %q", none.Description)
	}
}

func TestFailNote(t *testing.T) {
	err := errors.New(`Patch "https://discord.com/api/v9/webhooks/1565064/aW50ZXJhY3Rpb24/messages/@original": context deadline exceeded`)
	want := "-# couldn't produce image: Patch discord: context deadline exceeded"
	if got := FailNote(err); got != want {
		t.Errorf("FailNote() = %q, want %q", got, want)
	}
	if got := FailNote(errors.New("renderer down")); got != "-# couldn't produce image: renderer down" {
		t.Errorf("FailNote() = %q", got)
	}
}

func TestLanded(t *testing.T) {
	msg := &discordgo.Message{Attachments: []*discordgo.MessageAttachment{{Filename: "daily_124.webp"}}}
	fetch := func() (*discordgo.Message, error) { return msg, nil }
	if got, ok := Landed(fetch, "daily_124.webp"); !ok || got != msg {
		t.Error("the upload landed but wasn't found")
	}
	if _, ok := Landed(fetch, "daily_125.webp"); ok {
		t.Error("another file counted as landed")
	}
	if _, ok := Landed(func() (*discordgo.Message, error) { return nil, errors.New("down") }, "daily_124.webp"); ok {
		t.Error("a failed fetch counted as landed")
	}
}
