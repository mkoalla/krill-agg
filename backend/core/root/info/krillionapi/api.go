package krillionapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
func (s Stats) Percentile(score int) float64 {
	if s.Sample <= 0 || len(s.Dist) == 0 {
		return 0
	}
	bucket := min(max(score, 0)/BUCKET_WIDTH, len(s.Dist)-1)
	below := 0.0
	for _, count := range s.Dist[:bucket] {
		below += count
	}
	into := float64(score-bucket*BUCKET_WIDTH) / BUCKET_WIDTH
	below += s.Dist[bucket] * min(into, 1)
	return 100 * below / float64(s.Sample)
}
