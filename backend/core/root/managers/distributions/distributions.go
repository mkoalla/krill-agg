// Package distributions keeps krillion's /complete answer (player count and
// score histogram) for each day, so a past day's card can be drawn after
// krillion has moved on. Daily reports save today's; older days were
// imported from krillion's archive. Some early days have no histogram at
// all (sample 0, no dist); those are kept too, as a record that there's none.
package distributions

import (
	fd "krill-agg/root/datas/filedict"
	"krill-agg/root/info/krillionapi"
)

type Stats = krillionapi.Stats

// krillion # -> its /complete answer
type Manager struct {
	data *fd.FileDict[int, Stats]
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
	return man.data.SetSync(day, s)
}

// whether a saved answer has a histogram to draw
func HasDist(s Stats) bool { return s.Sample > 0 && len(s.Dist) > 0 }
