package scores

import (
	"cmp"
	"errors"
	"krill-agg/root/krillion"
	"slices"
	"sync"
)

type uid = string
type guildID = string
type PlayerScore = krillion.PlayerScore

type Outcome int

const (
	Ignored  Outcome = iota // not a krillion message
	Rejected                // failed parsing or validation; see Result.Err
	Stored                  // new entry saved
	Conflict                // user already has an entry for this day; nothing saved
)

type Result struct {
	Outcome  Outcome
	Score    PlayerScore // the incoming score (Stored/Conflict)
	Existing PlayerScore // the current entry (Conflict only)
	Err      error       // Rejected only
}

type Entry struct {
	UserID string
	Score  PlayerScore
}

// in-memory guild -> day -> user -> score.
// the guild always comes from where the message/interaction happened, never user input
type ScoreManager struct {
	mu     sync.RWMutex
	guilds map[guildID]map[int]map[uid]PlayerScore
}

func New() *ScoreManager {
	return &ScoreManager{guilds: make(map[guildID]map[int]map[uid]PlayerScore)}
}

// caller holds the write lock
func (man *ScoreManager) dayFor(guild guildID, day int) map[uid]PlayerScore {
	days, ok := man.guilds[guild]
	if !ok {
		days = make(map[int]map[uid]PlayerScore)
		man.guilds[guild] = days
	}
	players, ok := days[day]
	if !ok {
		players = make(map[uid]PlayerScore)
		days[day] = players
	}
	return players
}

// runs a message through the parser and stores it if it's new
func (man *ScoreManager) Ingest(guild guildID, id uid, content string) Result {
	p, err := krillion.FromMessage(content)
	if errors.Is(err, krillion.ErrNotKrillion) {
		return Result{Outcome: Ignored}
	}
	if err != nil {
		return Result{Outcome: Rejected, Err: err}
	}

	man.mu.Lock()
	defer man.mu.Unlock()
	day := man.dayFor(guild, p.Day)
	if existing, ok := day[id]; ok {
		return Result{Outcome: Conflict, Score: p, Existing: existing}
	}
	day[id] = p
	return Result{Outcome: Stored, Score: p}
}

// overwrites unconditionally (used after a user confirms an update)
func (man *ScoreManager) Set(guild guildID, id uid, p PlayerScore) {
	man.mu.Lock()
	defer man.mu.Unlock()
	man.dayFor(guild, p.Day)[id] = p
}

func (man *ScoreManager) Get(guild guildID, day int, id uid) (PlayerScore, bool) {
	man.mu.RLock()
	defer man.mu.RUnlock()
	p, ok := man.guilds[guild][day][id]
	return p, ok
}

// snapshot of a day sorted by score (desc), ok=false when nobody has played.
// an empty map left at the key is pruned
func (man *ScoreManager) Day(guild guildID, day int) ([]Entry, bool) {
	man.mu.Lock()
	defer man.mu.Unlock()
	days := man.guilds[guild]
	players, ok := days[day]
	if !ok {
		return nil, false
	}
	if len(players) == 0 {
		delete(days, day)
		if len(days) == 0 {
			delete(man.guilds, guild)
		}
		return nil, false
	}
	entries := make([]Entry, 0, len(players))
	for id, p := range players {
		entries = append(entries, Entry{UserID: id, Score: p})
	}
	slices.SortFunc(entries, func(a, b Entry) int {
		if a.Score.Score != b.Score.Score {
			return b.Score.Score - a.Score.Score
		}
		return cmp.Compare(a.UserID, b.UserID)
	})
	return entries, true
}
