package dailygen

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// where the renderer reads avatars and writes cards, under the shared data
// mount. one card's files sit in DIR/<guild>/<day>/: its images folder (made
// fresh per render, removed after) and daily_<day>.png
const DIR = "dailygen"

var client = &http.Client{Timeout: 60 * time.Second}

// the renderer's compose service on the krill-agg network. a var so `make dev`
// can point it at a local server with -ldflags -X (and tests at theirs)
var baseURL = "http://dailygen:40404"

type User struct {
	Score int `json:"score"`
}

// avatars are implicit: Users[i] is drawn from <guild>/<day>/<Images>/<i>.png
// under DIR. Icon names the guild's icon in that same folder; empty draws none
type Request struct {
	Guild  string `json:"guild"`
	Title  string `json:"title"`
	Day    int    `json:"day"`
	Images string `json:"images"` // this render's own folder, so renders never share avatars
	// nil: nobody played (no average, pin or top score)
	Mean *float64 `json:"mean"`
	// empty: krillion has no distribution for the day (a flat line, no percentile)
	Dist       []float64 `json:"dist"`
	Percentile *float64  `json:"percentile"`
	Users      []User    `json:"users"`
	Icon       string    `json:"icon,omitempty"`
	Sample     int       `json:"sample"`               // krillion's player count that day
	Population string    `json:"population,omitempty"` // "better than x% of <this>"; default today's players
	// avatars a clashing stack grows before wrapping back down (modulo)
	MaxPush int `json:"max_push"`
}

// how high clashing avatars stack before wrapping
const MAX_PUSH = 4

type response struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// renders the card; path is relative to DIR
func Render(req Request) (path string, err error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	resp, err := client.Post(baseURL+"/render", "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return "", err
	}
	var out response
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("renderer %d: %s", resp.StatusCode, bytes.TrimSpace(body))
	}
	if resp.StatusCode != http.StatusOK {
		if out.Error == "" {
			out.Error = http.StatusText(resp.StatusCode)
		}
		return "", fmt.Errorf("renderer: %s", out.Error)
	}
	if out.Path == "" {
		return "", errors.New("renderer returned no path")
	}
	return out.Path, nil
}
