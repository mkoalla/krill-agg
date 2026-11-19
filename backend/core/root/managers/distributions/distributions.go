// Package distributions keeps krillion's /complete answer (player count and
// score histogram) for each day, so a past day's card can be drawn after
// krillion has moved on. Daily reports save today's; older days were
// imported from krillion's archive. Some early days have no histogram at
// all (sample 0, no dist); those are kept too, as a record that there's none.
package distributions

import (
	fd "krill-agg/root/datas/filedict"
	"krill-agg/root/info/krillionapi"
	"slices"
	"sync"
)

type Stats = krillionapi.Stats

// krillion # -> its /complete answer
type Manager struct {
	data *fd.FileDict[int, Stats]

	// every saved day added up (see AllTime): worked out the first time it's
	// asked for, then kept up to date by Save. held across a save and its
	// update so neither is missed
	mu     sync.Mutex
	summed *Stats
}

// adds (sign 1) or takes away (-1) a day's histogram from the running total
func addTo(t *Stats, s Stats, sign float64) {
	if !HasDist(s) {
		return
	}
	if len(t.Dist) < len(s.Dist) {
		t.Dist = append(t.Dist, make([]float64, len(s.Dist)-len(t.Dist))...)
	}
	for b, count := range s.Dist {
		t.Dist[b] += sign * count
	}
	t.Sample += int(sign) * s.Sample
}

func validate(day *int, s *Stats) bool { return *day > 0 && s.Sample >= 0 }

func New(path string) (*Manager, error) {
	data, err := fd.New(path, validate)
	if err != nil {
		return nil, err
	}
	return &Manager{data: data}, nil
}

func (man *Manager) Close() error { return man.data.Close() }

// the saved answer for krillion #day; ok=false if it was never saved
func (man *Manager) Get(day int) (Stats, bool) {
	return man.data.Get(day)
}

// saves (or replaces) the day's answer
func (man *Manager) Save(day int, s Stats) error {
	man.mu.Lock()
	defer man.mu.Unlock()
	old, had := man.data.Get(day)
	if err := man.data.SetSync(day, s); err != nil {
		return err
	}
	if man.summed != nil {
		if had {
			addTo(man.summed, old, -1)
		}
		addTo(man.summed, s, 1)
	}
	return nil
}

// every saved day's histogram added up (days without one left out), with
// their player counts summed as Sample. the days are read once, then Save
// keeps it current. ok=false while nothing has a histogram
func (man *Manager) AllTime() (Stats, bool) {
	man.mu.Lock()
	defer man.mu.Unlock()
	if man.summed == nil {
		man.summed = &Stats{}
		for _, s := range man.data.Iter() {
			addTo(man.summed, s, 1)
		}
	}
	out := Stats{Sample: man.summed.Sample, Dist: slices.Clone(man.summed.Dist)}
	return out, HasDist(out)
}

// whether a saved answer has a histogram to draw
func HasDist(s Stats) bool { return s.Sample > 0 && len(s.Dist) > 0 }
