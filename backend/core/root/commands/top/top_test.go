package top

import (
	"krill-agg/root/managers/leaderboard"
	"strings"
	"testing"
	"time"
)

func board() []Row {
	rows := make([]Row, 12)
	for j := range rows {
		rows[j] = Row{Rank: j + 1, UserID: string(rune('a' + j)), Value: float64(415 - 30*j), Day: 124 - j, Emojis: "🦑🦑🦑🐟🦑🦑🏮"}
	}
	return rows
}

func TestColumns(t *testing.T) {
	rows := board()
	fields := columns(BOARD_INFO[leaderboard.BEST], pageOf(rows, 0)[:2], rows)
	if len(fields) != 2 || !fields[0].Inline || !fields[1].Inline || fields[1].Name != "Score" {
		t.Fatalf("fields = %+v", fields)
	}
	// ranks padded to "#12", values to the widest (415 vs 85: 3 wide)
	if want := "`#1 ` <@a>\n`#2 ` <@b>"; fields[0].Value != want {
		t.Errorf("player column = %q, want %q", fields[0].Value, want)
	}
	if want := "`415` `#124 🦑🦑🦑🐟🦑🦑🏮`\n`385` `#123 🦑🦑🦑🐟🦑🦑🏮`"; fields[1].Value != want {
		t.Errorf("value column = %q, want %q", fields[1].Value, want)
	}
	if got := valueCell(BOARD_INFO[leaderboard.BEST], rows[11], 3); got != "` 85` `#113 🦑🦑🦑🐟🦑🦑🏮`" {
		t.Errorf("short value not padded: %q", got)
	}
	if got := valueCell(BOARD_INFO[leaderboard.WORST], Row{Value: 0, Day: 7, Emojis: "⬛"}, 3); got != "`  0` `#7   ⬛`" {
		t.Errorf("short day not padded: %q", got)
	}
	avg := valueCell(BOARD_INFO[leaderboard.AVERAGE], Row{Value: 207.5}, 5)
	if avg != "`207.5`" {
		t.Errorf("average cell = %q", avg)
	}
}

func TestPaging(t *testing.T) {
	rows := board()
	if n := len(pageOf(rows, 0)); n != 10 {
		t.Errorf("page 1 has %d rows", n)
	}
	if n := len(pageOf(rows, 1)); n != 2 {
		t.Errorf("page 2 has %d rows", n)
	}
	if n := len(pageOf(rows, 5)); n != 0 {
		t.Errorf("past the end has %d rows", n)
	}
	for players, want := range map[int]int{0: 1, 1: 1, 10: 1, 11: 2, 25: 3} {
		if got := pageCount(players); got != want {
			t.Errorf("pageCount(%d) = %d, want %d", players, got, want)
		}
	}
}

func TestSelfLine(t *testing.T) {
	rows := board()
	line, ok := selfLine(BOARD_INFO[leaderboard.BEST], rows, "c")
	if !ok || line != "**Best Scores** — `#3` of 12 · `355` `#122 🦑🦑🦑🐟🦑🦑🏮`" {
		t.Errorf("selfLine = %q %v", line, ok)
	}
	if _, ok := selfLine(BOARD_INFO[leaderboard.PLAYED], rows, "zz"); ok {
		t.Error("a non-player got a line")
	}
	if !strings.Contains(disclaimer(fixedNow).Value, "<t:") {
		t.Error("disclaimer has no timestamp")
	}
}

var fixedNow = time.Date(2026, 11, 16, 15, 10, 0, 0, time.UTC)

func TestBlacklistEmbed(t *testing.T) {
	if e := blacklistEmbed(nil); e.Description != "Nobody is blacklisted." {
		t.Errorf("empty = %q", e.Description)
	}
	if e := blacklistEmbed([]string{"1", "2"}); e.Description != "<@1>\n<@2>" || e.Title == "" {
		t.Errorf("list = %+v", e)
	}
}

func TestBoardTitle(t *testing.T) {
	if got := boardTitle("Best Scores", leaderboard.WEEK, "Krill Club"); got != "🏆 Best Scores (1 Week) in Krill Club" {
		t.Errorf("boardTitle() = %q", got)
	}
	if got := boardTitle("Your Ranks", leaderboard.ALLTIME, "Krill Club"); got != "🏆 Your Ranks (All-time) in Krill Club" {
		t.Errorf("boardTitle() = %q", got)
	}
}
