// Package krills keeps each day's krillion answers (the 100-point tier) and
// krillion's quips on them, one list per question. krillion only reveals
// today's answer key, so a day is saved when a daily report (or /krill on
// the day) fetches it, and read back from krills.db after that. a saved day
// is final: krillion's answers for it don't change
package krills

import (
	"errors"
	fd "krill-agg/root/datas/filedict"
	"krill-agg/root/info/krillionapi"
	"krill-agg/root/krillion"
	"slices"
	"strings"
)

var ErrNotToday = errors.New("krillion only reveals today's answers")

// one krillion answer; Quip is "" when krillion has none for it
type Krill struct {
	Answer string `json:"answer"`
	Quip   string `json:"quip,omitempty"`
}

// krillion # -> per question (in order), its krillion answers (maybe none)
type Manager struct {
	data  *fd.FileDict[int, [][]Krill]
	fetch func(date string) (krillionapi.Reveal, error)
	today func() int
}

func validate(day *int, _ *[][]Krill) bool { return *day > 0 }

func New(path string) (*Manager, error) {
	data, err := fd.New(path, validate)
	if err != nil {
		return nil, err
	}
	return &Manager{data: data, fetch: krillionapi.FetchReveal, today: krillion.Today}, nil
}

func (man *Manager) Close() error { return man.data.Close() }

// the saved answers for krillion #day; ok=false if they were never collected
func (man *Manager) Get(day int) ([][]Krill, bool) {
	qs, ok := man.data.Get(day)
	return slices.Clone(qs), ok
}

// the krillion answers out of a whole answer key, per question
func Pick(r krillionapi.Reveal) [][]Krill {
	out := make([][]Krill, len(r.Prompts))
	for j, p := range r.Prompts {
		out[j] = []Krill{}
		for _, a := range p.Answers {
			if a.Score != krillionapi.KRILLION_SCORE {
				continue
			}
			k := Krill{Answer: a.Answer}
			// absent, null and blank all mean no quip
			if a.Quip != nil {
				k.Quip = strings.TrimSpace(*a.Quip)
			}
			out[j] = append(out[j], k)
		}
	}
	return out
}

// the answers for krillion #day: saved ones if any, else fetched and saved,
// which only works for today (ErrNotToday otherwise)
func (man *Manager) Fetch(day int) ([][]Krill, error) {
	if qs, ok := man.Get(day); ok {
		return qs, nil
	}
	if day != man.today() {
		return nil, ErrNotToday
	}
	r, err := man.fetch(krillion.DateOf(day))
	if err != nil {
		return nil, err
	}
	if r.Date != krillion.DateOf(day) || len(r.Prompts) == 0 {
		return nil, errors.New("krillion sent a different day's answers, or none")
	}
	qs := Pick(r)
	if err := man.data.SetSync(day, qs); err != nil {
		return nil, err
	}
	return qs, nil
}
