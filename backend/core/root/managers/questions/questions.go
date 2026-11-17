// Package questions keeps each day's seven krillion questions. krillion only
// serves today's, so they're saved when a daily report fetches them and read
// back from questions.db for any day after that.
package questions

import (
	"errors"
	fd "krill-agg/root/datas/filedict"
	"krill-agg/root/info/krillionapi"
	"slices"
)

var ErrNotToday = errors.New("krillion only shares today's questions")

// krillion # -> its questions, in order
type Manager struct {
	data  *fd.FileDict[int, []string]
	fetch func() (krillionapi.Puzzle, error)
}

func validate(day *int, qs *[]string) bool { return *day > 0 && len(*qs) > 0 }

func New(path string) (*Manager, error) {
	data, err := fd.New(path, validate)
	if err != nil {
		return nil, err
	}
	return &Manager{data: data, fetch: krillionapi.FetchPuzzle}, nil
}

func (man *Manager) Close() error { return man.data.Close() }

// the saved questions for krillion #day; ok=false if none were ever fetched
func (man *Manager) Get(day int) ([]string, bool) {
	qs, ok := man.data.Get(day)
	return slices.Clone(qs), ok
}

// asks krillion for today's questions and saves them, if day is today
// (ErrNotToday otherwise). always asks, so a fix on their side is picked up
func (man *Manager) Fetch(day int) ([]string, error) {
	p, err := man.fetch()
	if err != nil {
		return nil, err
	}
	if p.DayNumber != day {
		return nil, ErrNotToday
	}
	qs := make([]string, len(p.Prompts))
	for j, prompt := range p.Prompts {
		qs[j] = prompt.Text
	}
	if len(qs) == 0 {
		return nil, errors.New("krillion sent no questions")
	}
	if err := man.data.SetSync(day, qs); err != nil {
		return nil, err
	}
	return qs, nil
}
