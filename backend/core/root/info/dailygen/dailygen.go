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

// where the renderer reads avatars and writes cards, under the shared data mount
const DIR = "daily"

var client = &http.Client{Timeout: 60 * time.Second}

// the renderer's compose service on the krill-agg network. a var so `make dev`
// can point it at a local server with -ldflags -X (and tests at theirs)
var baseURL = "http://dailygen:40404"

type User struct {
	Score int `json:"score"`
}

// avatars are implicit: Users[i] is drawn from <guild>/images/<i>.png under DIR.
// Icon names the guild's icon in that same folder; empty draws none
type Request struct {
	Guild      string    `json:"guild"`
	Title      string    `json:"title"`
	Day        int       `json:"day"`
	Mean       float64   `json:"mean"`
	Dist       []float64 `json:"dist"`
	Percentile float64   `json:"percentile"`
	Users      []User    `json:"users"`
	Icon       string    `json:"icon,omitempty"`
}

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
