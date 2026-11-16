package daily

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestPostAllCapsWorkers(t *testing.T) {
	var running, peak atomic.Int32
	channels := make([]string, 12)
	for j := range channels {
		channels[j] = fmt.Sprint(j)
	}
	results := postAll(channels, 5, func(channel string) (string, error) {
		n := running.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		running.Add(-1)
		if channel == "3" {
			return "", errors.New("boom")
		}
		return "link" + channel, nil
	})
	if p := peak.Load(); p != 5 {
		t.Errorf("peak concurrency %d, want 5", p)
	}
	for j, r := range results {
		if j == 3 {
			if r.err == nil {
				t.Error("channel 3 should have failed")
			}
			continue
		}
		if r.link != "link"+channels[j] || r.err != nil {
			t.Errorf("result %d = %+v, out of order or failed", j, r)
		}
	}
}
