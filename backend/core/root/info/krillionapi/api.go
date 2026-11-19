package krillionapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const (
	BASE_URL = "https://krillion.io/api"
	// width of each of the 20 histogram buckets (0-700)
	BUCKET_WIDTH = 35
	// sent when only the distribution is wanted; the middle keeps it neutral
	MIDDLE_SCORE = 350
)

var client = &http.Client{Timeout: 15 * time.Second}

type Stats struct {
	Sample     int       `json:"sample"`
	BetterThan float64   `json:"betterThan"` // percent, for the score that was sent
	Dist       []float64 `json:"dist"`       // player counts per BUCKET_WIDTH bucket
}

func decode(resp *http.Response, into any) error {
	defer resp.Body.Close() //nolint
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("krillion api %d: %s", resp.StatusCode, bytes.TrimSpace(body))
	}
	return json.Unmarshal(body, into)
}

// asks where score would land on today's puzzle (date from krillion.DateOf; past
// dates are refused). a fresh random player id each time and
// record:false, so the lookup isn't kept as a result
func Complete(date string, score int) (Stats, error) {
	payload, err := json.Marshal(map[string]any{
		"date":     date,
		"playerId": uuid.NewString(),
		"score":    score,
		"record":   false,
	})
	if err != nil {
		return Stats{}, err
	}
	resp, err := client.Post(BASE_URL+"/today/complete", "application/json", bytes.NewReader(payload))
	if err != nil {
		return Stats{}, err
	}
	var s Stats
	return s, decode(resp, &s)
}

// percent of players below score, estimated from the histogram alone:
// every bucket under it, plus a linear share of its own bucket
func (s Stats) Percentile(score float64) float64 {
	if s.Sample <= 0 || len(s.Dist) == 0 {
		return 0
	}
	score = max(score, 0)
	bucket := min(int(score)/BUCKET_WIDTH, len(s.Dist)-1)
	below := 0.0
	for _, count := range s.Dist[:bucket] {
		below += count
	}
	into := (score - float64(bucket*BUCKET_WIDTH)) / BUCKET_WIDTH
	below += s.Dist[bucket] * min(into, 1)
	return 100 * below / float64(s.Sample)
}

// how many players the straight line inside score's bucket could misplace, if
// they thin out (or bunch up) across it instead of spreading evenly. the
// density at each edge is taken as the average of the buckets either side;
// under a density running straight between those, the linear guess is off by
// |edge difference|·t(1-t)/2 players at t of the way through. nothing at the
// edges, where the histogram is exact, or in an empty bucket
func Misplaced(dist []float64, score float64) float64 {
	if len(dist) == 0 {
		return 0
	}
	score = max(score, 0)
	b := min(int(score)/BUCKET_WIDTH, len(dist)-1)
	if dist[b] <= 0 {
		return 0
	}
	t := min((score-float64(b*BUCKET_WIDTH))/BUCKET_WIDTH, 1)
	neighbor := func(j int) float64 {
		if j < 0 || j >= len(dist) {
			return dist[b]
		}
		return dist[j]
	}
	low, high := (neighbor(b-1)+dist[b])/2, (dist[b]+neighbor(b+1))/2
	return math.Abs(high-low) * t * (1 - t) / 2
}

// a percentile to two decimals, floored so nobody is rounded up past where
// they are: "95.65"
func FormatPercent(p float64) string {
	return strconv.FormatFloat(math.Floor(p*100+1e-9)/100, 'f', 2, 64)
}

// Percentile, formatted
func (s Stats) PercentileText(score float64) string {
	return FormatPercent(s.Percentile(score))
}

// Percentile backwards: the score a share q (0-1) of the histogram is under,
// every bucket below it plus a linear share of its own. ok=false with nobody
// in it
func Quantile(dist []float64, q float64) (float64, bool) {
	total := 0.0
	for _, count := range dist {
		total += count
	}
	if total <= 0 {
		return 0, false
	}
	want, below := q*total, 0.0
	for b, count := range dist {
		if count > 0 && below+count >= want {
			return float64(b*BUCKET_WIDTH) + (want-below)/count*BUCKET_WIDTH, true
		}
		below += count
	}
	return float64(len(dist) * BUCKET_WIDTH), true
}

// one of the day's seven questions
type Prompt struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// GET /today: the puzzle being played right now
type Puzzle struct {
	Date      string   `json:"date"`
	DayNumber int      `json:"dayNumber"`
	Prompts   []Prompt `json:"prompts"`
}

// the puzzle being played right now (krillion only serves today's)
func FetchPuzzle() (Puzzle, error) {
	resp, err := client.Get(BASE_URL + "/today")
	if err != nil {
		return Puzzle{}, err
	}
	var p Puzzle
	return p, decode(resp, &p)
}

// one accepted answer from GET /reveal. Quip is krillion's note on it; most
// answers have none (the key is left out), so it's nil then
type RevealAnswer struct {
	Answer  string   `json:"answer"`
	Aliases []string `json:"aliases"`
	Tier    string   `json:"tier"`
	Score   int      `json:"score"`
	Quip    *string  `json:"quip"`
}

type RevealPrompt struct {
	ID      string         `json:"id"`
	Text    string         `json:"text"`
	Answers []RevealAnswer `json:"answers"`
}

// GET /reveal: a day's whole answer key, every accepted answer per question
type Reveal struct {
	Date    string         `json:"date"`
	Prompts []RevealPrompt `json:"prompts"`
}

// the top tier's score (krillion answers)
const KRILLION_SCORE = 100

// the answer key for date (krillion.DateOf). krillion serves today's
func FetchReveal(date string) (Reveal, error) {
	resp, err := client.Get(BASE_URL + "/reveal?date=" + url.QueryEscape(date))
	if err != nil {
		return Reveal{}, err
	}
	var r Reveal
	return r, decode(resp, &r)
}
