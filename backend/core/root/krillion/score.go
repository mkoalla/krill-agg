package krillion

import (
	"fmt"
	"strings"
)

// number of emoji choices in a complete Krillion result
const NumChoices = 7

type PlayerScore struct {
	Day     int
	Score   int
	Choices []string // one entry per emoji (may be multi-codepoint)

	// the first non-scoring cluster that ended the emoji chain, if the chain
	// was cut short by something other than whitespace/end of text.
	// only used to give a better error in validation.
	stoppedAt string
}

// value of each scoring emoji; keys are stored without variation selectors
var EmojiValues = map[string]int{
	"⬛": 0,
	"🫧": 10,
	"🤡": 15,
	"🐟": 30,
	"🦑": 60,
	"🏮": 85,
	"🌟": 100,
}

// strips text/emoji presentation selectors so "⬛️" and "⬛" are the same key
func normalize(emoji string) string {
	return strings.NewReplacer("︎", "", "️", "").Replace(emoji)
}

// score value of a single emoji, or -1 if it isn't a scoring emoji
func ValueOf(emoji string) int {
	v, ok := EmojiValues[normalize(emoji)]
	if !ok {
		return -1
	}
	return v
}

// "🦑🦑🦑🐟🦑🦑🏮"
func (p PlayerScore) Emojis() string {
	return strings.Join(p.Choices, "")
}

// "Krillion #124 🦐"
// the krill krillion signs its results with
const HEADER_EMOJI = "🦐"

func Header(day int) string {
	return fmt.Sprintf("Krillion #%d %s", day, HEADER_EMOJI)
}

// the result laid out like the game's own share text
func (p PlayerScore) Message() string {
	return fmt.Sprintf("%s\n%d\n\n%s", Header(p.Day), p.Score, p.Emojis())
}

// "415 🦑🦑🦑🐟🦑🦑🏮"
func (p PlayerScore) Format() string {
	return fmt.Sprintf("%d %s", p.Score, p.Emojis())
}
