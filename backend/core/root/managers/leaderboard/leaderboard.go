// Package leaderboard ranks a guild's players four ways (best game, worst
// game, average, games played), one row per player, leaving out anyone on
// the guild's blacklist. Boards are built lazily:
// only when asked for, from scores.db, then kept in leaderboard.db and memory
// until the hour turns over (or a refetch).
package leaderboard

import (
	"cmp"
	"krill-agg/root/datas/boarddb"
	"krill-agg/root/managers/scores"
	"maps"
	"slices"
	"sync"
	"time"
)

type Row = boarddb.Row

const (
	BEST    = "best"
	WORST   = "worst"
	AVERAGE = "average"
	PLAYED  = "played"
)

// the order /top all shows them in
var BOARDS = []string{BEST, WORST, AVERAGE, PLAYED}

// where boards are built from
type source interface {
	// both leave out the excluded users in their query
	Guild(guild string, exclude []string) ([]scores.Entry, error)
	Players(guild string, exclude []string) ([]string, error)
}

type cached struct {
	rows []Row
	at   time.Time
}

type Manager struct {
	mu     sync.Mutex // also keeps two viewers from building the same board at once
	db     *boarddb.BoardDB
	scores source
	cache  map[string]map[string]*cached // guild -> board
	banned map[string]map[string]bool    // guild -> blacklisted users, loaded on first use
	now    func() time.Time
}

func New(path string, scores *scores.ScoreManager) (*Manager, error) {
	db, err := boarddb.New(path)
	if err != nil {
		return nil, err
	}
	return newManager(db, scores), nil
}

func newManager(db *boarddb.BoardDB, src source) *Manager {
	return &Manager{db: db, scores: src, cache: map[string]map[string]*cached{}, banned: map[string]map[string]bool{}, now: time.Now}
}

func (man *Manager) Close() error { return man.db.Close() }

// boards built before this are stale: they refresh at the start of every hour
func HourStart(t time.Time) time.Time { return t.Truncate(time.Hour) }

// when the boards next refresh
func NextUpdate(t time.Time) time.Time { return HourStart(t).Add(time.Hour) }

func (man *Manager) fresh(at time.Time) bool { return !at.Before(HourStart(man.now())) }

// the guild's board, ranked from 1, and when it was built. built only if
// there's nothing from this hour in memory or in leaderboard.db
func (man *Manager) Board(guild, board string) ([]Row, time.Time, error) {
	man.mu.Lock()
	defer man.mu.Unlock()
	if c, ok := man.cache[guild][board]; ok && man.fresh(c.at) {
		return c.rows, c.at, nil
	}
	if f, ok, err := man.db.LastFetch(guild, board); err != nil {
		return nil, time.Time{}, err
	} else if ok && man.fresh(f.At) {
		rows, err := man.db.All(guild, board)
		if err != nil {
			return nil, time.Time{}, err
		}
		man.remember(guild, board, rows, f.At)
		return rows, f.At, nil
	}
	return man.rebuildLocked(guild, board)
}

// caller holds the lock
func (man *Manager) rebuildLocked(guild, board string) ([]Row, time.Time, error) {
	banned, err := man.bannedLocked(guild)
	if err != nil {
		return nil, time.Time{}, err
	}
	entries, err := man.scores.Guild(guild, slices.Collect(maps.Keys(banned)))
	if err != nil {
		return nil, time.Time{}, err
	}
	// the query already left them out; this is just a second check
	entries = slices.DeleteFunc(entries, func(e scores.Entry) bool { return banned[e.UserID] })
	rows := Build(board, entries)
	at := man.now()
	if err := man.db.Replace(guild, board, rows, at); err != nil {
		return nil, time.Time{}, err
	}
	// ranks as stored, so a board read back from the db matches
	for j := range rows {
		rows[j].Rank = j + 1
	}
	man.remember(guild, board, rows, at)
	return rows, at, nil
}

func (man *Manager) remember(guild, board string, rows []Row, at time.Time) {
	if man.cache[guild] == nil {
		man.cache[guild] = map[string]*cached{}
	}
	man.cache[guild][board] = &cached{rows: rows, at: at}
}

// how many rows the guild's boards have (every board has one per player),
// without building one: a fresh board's count if there is one, else the
// players in scores.db who aren't blacklisted
func (man *Manager) Players(guild string) (int, error) {
	man.mu.Lock()
	defer man.mu.Unlock()
	for _, c := range man.cache[guild] {
		if man.fresh(c.at) {
			return len(c.rows), nil
		}
	}
	banned, err := man.bannedLocked(guild)
	if err != nil {
		return 0, err
	}
	ids, err := man.scores.Players(guild, slices.Collect(maps.Keys(banned)))
	if err != nil {
		return 0, err
	}
	n := 0 // the query already left them out; this is just a second check
	for _, id := range ids {
		if !banned[id] {
			n++
		}
	}
	return n, nil
}

// caller holds the lock
func (man *Manager) bannedLocked(guild string) (map[string]bool, error) {
	if b, ok := man.banned[guild]; ok {
		return b, nil
	}
	ids, err := man.db.Blacklist(guild)
	if err != nil {
		return nil, err
	}
	b := make(map[string]bool, len(ids))
	for _, id := range ids {
		b[id] = true
	}
	man.banned[guild] = b
	return b, nil
}

// the guild's blacklisted users, sorted
func (man *Manager) Blacklist(guild string) ([]string, error) {
	man.mu.Lock()
	defer man.mu.Unlock()
	banned, err := man.bannedLocked(guild)
	if err != nil {
		return nil, err
	}
	return slices.Sorted(maps.Keys(banned)), nil
}

func (man *Manager) IsBlacklisted(guild, user string) (bool, error) {
	man.mu.Lock()
	defer man.mu.Unlock()
	banned, err := man.bannedLocked(guild)
	return banned[user], err
}

// adds or removes the user from the guild's blacklist. a change drops the
// guild's boards (memory and db), so the next view rebuilds them without
// (or with) the user. changed=false if they already were that way
func (man *Manager) SetBlacklisted(guild, user string, on bool) (changed bool, err error) {
	man.mu.Lock()
	defer man.mu.Unlock()
	changed, err = man.db.SetBlacklisted(guild, user, on)
	if err != nil || !changed {
		return changed, err
	}
	delete(man.banned, guild)
	delete(man.cache, guild)
	return true, nil
}

// drops the guild's cached boards and rebuilds every one now
func (man *Manager) Refetch(guild string) error {
	man.mu.Lock()
	defer man.mu.Unlock()
	delete(man.cache, guild)
	for _, board := range BOARDS {
		if _, _, err := man.rebuildLocked(guild, board); err != nil {
			return err
		}
	}
	return nil
}

type tally struct {
	user        string
	best, worst scores.PlayerScore
	sum, count  int
}

func (t *tally) average() float64 { return float64(t.sum) / float64(t.count) }

// one row per player, ranked. ties: earlier game (best/worst), more games
// (average), higher average (played), then user id so the order is stable
func Build(board string, entries []scores.Entry) []Row {
	byUser := map[string]*tally{}
	for _, e := range entries {
		t, ok := byUser[e.UserID]
		if !ok {
			t = &tally{user: e.UserID, best: e.Score, worst: e.Score}
			byUser[e.UserID] = t
		}
		p := e.Score
		if p.Score > t.best.Score || (p.Score == t.best.Score && p.Day < t.best.Day) {
			t.best = p
		}
		if p.Score < t.worst.Score || (p.Score == t.worst.Score && p.Day < t.worst.Day) {
			t.worst = p
		}
		t.sum += p.Score
		t.count++
	}
	tallies := make([]*tally, 0, len(byUser))
	for _, t := range byUser {
		tallies = append(tallies, t)
	}

	var order func(a, b *tally) int
	var row func(t *tally) Row
	switch board {
	case BEST:
		order = func(a, b *tally) int {
			return cmp.Or(cmp.Compare(b.best.Score, a.best.Score), cmp.Compare(a.best.Day, b.best.Day))
		}
		row = func(t *tally) Row { return gameRow(t.user, t.best) }
	case WORST:
		order = func(a, b *tally) int {
			return cmp.Or(cmp.Compare(a.worst.Score, b.worst.Score), cmp.Compare(a.worst.Day, b.worst.Day))
		}
		row = func(t *tally) Row { return gameRow(t.user, t.worst) }
	case AVERAGE:
		order = func(a, b *tally) int {
			return cmp.Or(cmp.Compare(b.average(), a.average()), cmp.Compare(b.count, a.count))
		}
		row = func(t *tally) Row { return Row{UserID: t.user, Value: t.average()} }
	case PLAYED:
		order = func(a, b *tally) int {
			return cmp.Or(cmp.Compare(b.count, a.count), cmp.Compare(b.average(), a.average()))
		}
		row = func(t *tally) Row { return Row{UserID: t.user, Value: float64(t.count)} }
	default:
		return nil
	}
	slices.SortFunc(tallies, func(a, b *tally) int {
		return cmp.Or(order(a, b), cmp.Compare(a.user, b.user))
	})
	rows := make([]Row, len(tallies))
	for j, t := range tallies {
		rows[j] = row(t)
		rows[j].Rank = j + 1
	}
	return rows
}

func gameRow(user string, p scores.PlayerScore) Row {
	return Row{UserID: user, Value: float64(p.Score), Day: p.Day, Emojis: p.Emojis()}
}
