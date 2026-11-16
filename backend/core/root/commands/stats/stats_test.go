package stats

import (
	"krill-agg/root/krillion"
	"testing"
)

func TestLine(t *testing.T) {
	cases := []struct {
		choices []string
		score   int
		want    string
	}{
		{[]string{"🦑", "🦑", "🦑", "🐟", "🦑", "🦑", "🏮"}, 415, "`🦑🦑🦑🐟🦑🦑🏮` - `415` - <@1>"},
		{[]string{"⬛", "🫧", "🤡"}, 25, "`⬛🫧🤡    ` - ` 25` - <@1>"},
		{[]string{"⬛️", "⬛"}, 0, "`⬛️⬛     ` - `  0` - <@1>"}, // FE0F doesn't count as a column
	}
	for _, c := range cases {
		got := line("1", krillion.PlayerScore{Score: c.score, Choices: c.choices})
		if got != c.want {
			t.Errorf("line() = %q, want %q", got, c.want)
		}
	}
}
