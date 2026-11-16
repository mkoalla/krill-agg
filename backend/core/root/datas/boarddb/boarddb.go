package boarddb

import (
	"database/sql"
	"fmt"
	"krill-agg/root/datas/sqlite"
	"slices"
	"time"
)

const FILE_TIMEOUT time.Duration = 5000 * time.Millisecond

// see scoredb
const PRAGMAS = "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"

// one table per board, all the same shape. rows are keyed (guild, rank), so a
// guild's board sits together on disk and a page is one key-range read
var BOARDS = []string{"best", "worst", "average", "played"}

// one ranked row. Day/Emojis are only set on best and worst
type Row struct {
	Rank   int
	UserID string
	Value  float64
	Day    int
	Emojis string
}

// when a guild's board was last built, and how many rows it had
type Fetch struct {
	At   time.Time
	Rows int
}

type BoardDB struct {
	file *sqlite.SQLiteDB
}

func New(path string) (*BoardDB, error) {
	file, err := sqlite.New(path+PRAGMAS, FILE_TIMEOUT)
	if err != nil {
		return nil, err
	}
	for _, board := range BOARDS {
		if _, err := file.Exec(`CREATE TABLE IF NOT EXISTS ` + board + ` (
			guild_id TEXT    NOT NULL,
			rank     INTEGER NOT NULL,
			user_id  TEXT    NOT NULL,
			value    REAL    NOT NULL,
			day      INTEGER,
			emojis   TEXT,
			PRIMARY KEY (guild_id, rank)
		) WITHOUT ROWID;`); err != nil {
			return nil, err
		}
	}
	// a row flags the user as left off the guild's leaderboards
	if _, err := file.Exec(`CREATE TABLE IF NOT EXISTS blacklist (
		guild_id TEXT NOT NULL,
		user_id  TEXT NOT NULL,
		PRIMARY KEY (guild_id, user_id)
	) WITHOUT ROWID;`); err != nil {
		return nil, err
	}
	if _, err := file.Exec(`CREATE TABLE IF NOT EXISTS fetches (
		guild_id   TEXT    NOT NULL,
		board      TEXT    NOT NULL,
		fetched_at INTEGER NOT NULL,
		rows       INTEGER NOT NULL,
		PRIMARY KEY (guild_id, board)
	) WITHOUT ROWID;`); err != nil {
		return nil, err
	}
	return &BoardDB{file: file}, nil
}

func (db *BoardDB) Close() error { return db.file.Close() }

// table names can't be bound, so they only ever come from BOARDS
func checkBoard(board string) error {
	if !slices.Contains(BOARDS, board) {
		return fmt.Errorf("no leaderboard %q", board)
	}
	return nil
}

// replaces the guild's board with rows (ranked in order, from 1) and records
// the fetch. one transaction, so a reader sees the old board or the new one
func (db *BoardDB) Replace(guild, board string, rows []Row, at time.Time) error {
	if err := checkBoard(board); err != nil {
		return err
	}
	args := make([]any, 0, 6*len(rows))
	for j, r := range rows {
		var day, emojis any
		if r.Emojis != "" {
			day, emojis = r.Day, r.Emojis
		}
		args = append(args, guild, j+1, r.UserID, r.Value, day, emojis)
	}
	return db.file.Tx(func(tx *sqlite.Tx) error {
		if _, err := tx.Exec(`DELETE FROM `+board+` WHERE guild_id = ?;`, guild); err != nil {
			return err
		}
		if len(args) > 0 {
			if err := tx.InsertRows(`INSERT INTO `+board+` (guild_id, rank, user_id, value, day, emojis) VALUES`, 6, args); err != nil {
				return err
			}
		}
		_, err := tx.Exec(`INSERT INTO fetches (guild_id, board, fetched_at, rows) VALUES (?, ?, ?, ?)
			ON CONFLICT (guild_id, board) DO UPDATE SET fetched_at = excluded.fetched_at, rows = excluded.rows;`,
			guild, board, at.Unix(), len(rows))
		return err
	})
}

// the last fetch of the guild's board; ok=false if it was never built
func (db *BoardDB) LastFetch(guild, board string) (f Fetch, ok bool, err error) {
	iter, err := db.file.Query(`SELECT fetched_at, rows FROM fetches WHERE guild_id = ? AND board = ?;`, guild, board)
	if err != nil {
		return Fetch{}, false, err
	}
	defer iter.Close() //nolint
	var at int64
	ok, err = iter.NextScan(&at, &f.Rows)
	f.At = time.Unix(at, 0)
	return f, ok, err
}

// marks every board of the guild stale, so the next read rebuilds it
func (db *BoardDB) Invalidate(guild string) error {
	_, err := db.file.Exec(`DELETE FROM fetches WHERE guild_id = ?;`, guild)
	return err
}

// the guild's blacklisted users
func (db *BoardDB) Blacklist(guild string) ([]string, error) {
	iter, err := db.file.Query(`SELECT user_id FROM blacklist WHERE guild_id = ? ORDER BY user_id;`, guild)
	if err != nil {
		return nil, err
	}
	defer iter.Close() //nolint
	var ids []string
	for {
		var id string
		ok, err := iter.NextScan(&id)
		if err != nil {
			return nil, err
		}
		if !ok {
			return ids, nil
		}
		ids = append(ids, id)
	}
}

// flags (or unflags) the user and marks the guild's boards stale in one
// transaction; changed=false if they were already that way
func (db *BoardDB) SetBlacklisted(guild, user string, on bool) (changed bool, err error) {
	err = db.file.Tx(func(tx *sqlite.Tx) error {
		query := `INSERT INTO blacklist (guild_id, user_id) VALUES (?, ?) ON CONFLICT DO NOTHING;`
		if !on {
			query = `DELETE FROM blacklist WHERE guild_id = ? AND user_id = ?;`
		}
		res, err := tx.Exec(query, guild, user)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil || n == 0 {
			return err
		}
		changed = true
		_, err = tx.Exec(`DELETE FROM fetches WHERE guild_id = ?;`, guild)
		return err
	})
	return changed, err
}

// ranks first..last (inclusive, from 1) of the guild's board
func (db *BoardDB) Range(guild, board string, first, last int) ([]Row, error) {
	if err := checkBoard(board); err != nil {
		return nil, err
	}
	iter, err := db.file.Query(`SELECT rank, user_id, value, day, emojis FROM `+board+`
		WHERE guild_id = ? AND rank BETWEEN ? AND ? ORDER BY rank;`, guild, first, last)
	if err != nil {
		return nil, err
	}
	defer iter.Close() //nolint
	var rows []Row
	for {
		var r Row
		var day sql.NullInt64
		var emojis sql.NullString
		ok, err := iter.NextScan(&r.Rank, &r.UserID, &r.Value, &day, &emojis)
		if err != nil {
			return nil, err
		}
		if !ok {
			return rows, nil
		}
		r.Day, r.Emojis = int(day.Int64), emojis.String
		rows = append(rows, r)
	}
}

// the guild's whole board
func (db *BoardDB) All(guild, board string) ([]Row, error) {
	return db.Range(guild, board, 1, int(^uint32(0)>>1))
}
