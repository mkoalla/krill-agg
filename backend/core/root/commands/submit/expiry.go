package submit

import (
	"log"
	"sync"
	"time"
)

// how often the worker checks for expired prompts
const EXPIRY_TICK = time.Second

// new prompts are handed to the worker here
var expiries = make(chan *promptState, 64)

var workerOnce sync.Once

func startExpiryWorker() {
	workerOnce.Do(func() { go expiryWorker() })
}

// collects prompts as they arrive and, every tick, ignores the ones that ran
// out of time. prompts already resolved or superseded by a newer epoch are
// dropped without touching discord.
func expiryWorker() {
	ticker := time.NewTicker(EXPIRY_TICK)
	defer ticker.Stop()

	var pending []*promptState
	for {
		select {
		case state := <-expiries:
			pending = append(pending, state)
		case now := <-ticker.C:
			pending = sweep(pending, now)
		}
	}
}

func sweep(pending []*promptState, now time.Time) []*promptState {
	kept := pending[:0]
	for _, state := range pending {
		if state.resolved.Load() || !state.current() {
			continue
		}
		if !state.expired(now) {
			kept = append(kept, state)
			continue
		}
		if !state.resolve() {
			continue
		}
		if err := ignore(state, "Time's up, score ignored."); err != nil {
			log.Printf("expiring krill-update prompt %s: %v", state.promptID, err)
		}
	}
	clear(pending[len(kept):])
	return kept
}
