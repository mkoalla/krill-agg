package krillion

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

var (
	// header: case-insensitive, anywhere in the message, text before it is ignored
	headerRe = regexp.MustCompile(`(?i)krillion\s*#`)
	// day: immediately after the '#', the rest of that line (incl. any emoji) is discarded
	dayRe = regexp.MustCompile(`^\s*(\d+)`)
	// score: on the line immediately following the header line
	scoreRe = regexp.MustCompile(`^[ \t]*(\d+)`)
)

// stage 1: parse the sequential pieces of a krillion result out of free-form chat text
func Parse(content string) (PlayerScore, error) {
	rest, ok := parseHeader(content)
	if !ok {
		return PlayerScore{}, ErrNotKrillion
	}

	day, rest, err := parseDay(rest)
	if err != nil {
		return PlayerScore{}, err
	}

	score, rest, err := parseScore(rest)
	if err != nil {
		return PlayerScore{}, err
	}

	choices, stoppedAt, err := parseEmojis(rest)
	if err != nil {
		return PlayerScore{}, err
	}

	return PlayerScore{Day: day, Score: score, Choices: choices, stoppedAt: stoppedAt}, nil
}

// finds the krillion header anywhere in the text; text before/after it is irrelevant here
func parseHeader(content string) (rest string, ok bool) {
	loc := headerRe.FindStringIndex(content)
	if loc == nil {
		return "", false
	}
	return content[loc[1]:], true
}

// day number right after the '#'; whatever else is on that line (e.g. an emoji) is dropped
func parseDay(s string) (day int, rest string, err error) {
	m := dayRe.FindStringSubmatchIndex(s)
	if m == nil {
		return 0, "", &ParseError{"couldn't parse day"}
	}
	day, aerr := strconv.Atoi(s[m[2]:m[3]])
	if aerr != nil {
		return 0, "", &ParseError{"couldn't parse day"}
	}
	return day, restOfLine(s, m[1]), nil
}

// score on the line immediately following the header line
func parseScore(s string) (score int, rest string, err error) {
	m := scoreRe.FindStringSubmatchIndex(s)
	if m == nil {
		return 0, "", &ParseError{"couldn't parse score"}
	}
	score, aerr := strconv.Atoi(s[m[2]:m[3]])
	if aerr != nil {
		return 0, "", &ParseError{"couldn't parse score"}
	}
	return score, restOfLine(s, m[1]), nil
}

// drops whatever remains of the current line, returning the start of the next one
// (or "" if there is no next line)
func restOfLine(s string, from int) string {
	if idx := strings.IndexByte(s[from:], '\n'); idx >= 0 {
		return s[from+idx+1:]
	}
	return ""
}

// skips leading whitespace (including blank lines), then consumes the emoji chain
// cluster by cluster, stopping at the first non-scoring cluster
func parseEmojis(s string) (choices []string, stoppedAt string, err error) {
	s = strings.TrimLeft(s, " \t\r\n")
	for {
		cluster, next := nextCluster(s)
		if cluster == "" {
			break
		}
		if ValueOf(cluster) == -1 {
			// only flag things that look like emoji; trailing text like "gg" just ends the chain
			if r, _ := utf8.DecodeRuneInString(cluster); r > unicode.MaxASCII && !unicode.IsSpace(r) {
				stoppedAt = cluster
			}
			break
		}
		choices = append(choices, cluster)
		s = next
	}
	if len(choices) == 0 {
		return nil, "", &ParseError{"couldn't parse emojis"}
	}
	return choices, stoppedAt, nil
}

// splits one grapheme cluster (a whole emoji, however many codepoints) off the front of s
func nextCluster(s string) (cluster, rest string) {
	cluster, rest, _, _ = uniseg.FirstGraphemeClusterInString(s, -1)
	return
}
