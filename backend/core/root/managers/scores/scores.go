package scores

import (
	"cmp"
	"errors"
	"fmt"
	"krill-agg/root/datas/scoredb"
	"krill-agg/root/krillion"
	"slices"
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
	Failed                  // valid, but couldn't be saved; see Result.Err
)

type Result struct {
	Outcome  Outcome
	Score    PlayerScore // the incoming score (Stored/Conflict)
	Existing PlayerScore // the current entry (Conflict only)
	Err      error       // Rejected/Failed only
}

type Entry struct {
	UserID string
	Score  PlayerScore
}

// guild -> day -> user -> score, backed by sqlite.
// the guild always comes from where the message/interaction happened, never user input
type ScoreManager struct {
	db *scoredb.ScoreDB
}

func New(path string) (*ScoreManager, error) {
	db, err := scoredb.New(path)
	if err != nil {
		return nil, err
	}
	return &ScoreManager{db: db}, nil
}

func (man *ScoreManager) Close() error { return man.db.Close() }

func toRow(guild guildID, id uid, p PlayerScore) scoredb.Row {
	return scoredb.Row{GuildID: guild, UserID: id, Day: p.Day, Emojis: p.Emojis()}
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

	// insert-if-absent is atomic, so two quick posts can't both be stored
	inserted, err := man.db.Insert(toRow(guild, id, p))
	if err != nil {
		return Result{Outcome: Failed, Score: p, Err: err}
	}
	if inserted {
		return Result{Outcome: Stored, Score: p}
	}
	existing, ok, err := man.Get(guild, p.Day, id)
	if err != nil {
		return Result{Outcome: Failed, Score: p, Err: err}
	}
	if !ok {
		return Result{Outcome: Failed, Score: p, Err: errors.New("entry vanished after conflict")}
	}
	return Result{Outcome: Conflict, Score: p, Existing: existing}
}

// overwrites unconditionally (used after a user confirms an update)
func (man *ScoreManager) Set(guild guildID, id uid, p PlayerScore) error {
	return man.db.Upsert(toRow(guild, id, p))
}

func (man *ScoreManager) Get(guild guildID, day int, id uid) (PlayerScore, bool, error) {
	row, ok, err := man.db.Get(guild, id, day)
	if err != nil || !ok {
		return PlayerScore{}, false, err
	}
	p, err := krillion.Rebuild(row.Day, row.Emojis)
	if err != nil {
		return PlayerScore{}, false, fmt.Errorf("stored score for %s day %d: %v", id, day, err)
	}
	return p, true, nil
}

// everyone in a guild on a day, sorted by score (desc). empty when nobody has played
func (man *ScoreManager) Day(guild guildID, day int) ([]Entry, error) {
	rows, err := man.db.Day(guild, day)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(rows))
	for _, row := range rows {
		p, err := krillion.Rebuild(row.Day, row.Emojis)
		if err != nil {
			return nil, fmt.Errorf("stored score for %s day %d: %v", row.UserID, day, err)
		}
		entries = append(entries, Entry{UserID: row.UserID, Score: p})
	}
	slices.SortFunc(entries, func(a, b Entry) int {
		if a.Score.Score != b.Score.Score {
			return b.Score.Score - a.Score.Score
		}
		return cmp.Compare(a.UserID, b.UserID)
	})
	return entries, nil
}
