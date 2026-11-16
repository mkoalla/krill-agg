package scoredb

import (
	"krill-agg/root/datas/sqlite"
	"time"
)

const FILE_TIMEOUT time.Duration = 5000 * time.Millisecond

// wait on locks instead of failing with SQLITE_BUSY; WAL lets reads run during writes
const PRAGMAS = "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"

// one stored result. the score isn't kept; it's rebuilt from the emojis
type Row struct {
	GuildID string
	UserID  string
	Day     int
	Emojis  string
}

type ScoreDB struct {
	file *sqlite.SQLiteDB
}

func New(path string) (*ScoreDB, error) {
	file, err := sqlite.New(path+PRAGMAS, FILE_TIMEOUT)
	if err != nil {
		return nil, err
	}
	// pkey makes a result unique per user per day per guild;
	// the index serves /stats (everyone in a guild on a day)
	if _, err := file.Exec(`CREATE TABLE IF NOT EXISTS scores (
		guild_id TEXT NOT NULL,
		user_id  TEXT NOT NULL,
		day      INTEGER NOT NULL,
		emojis   TEXT NOT NULL,
		PRIMARY KEY (guild_id, user_id, day)
	);`); err != nil {
		return nil, err
	}
	if _, err := file.Exec(`CREATE INDEX IF NOT EXISTS scores_guild_day ON scores (guild_id, day);`); err != nil {
		return nil, err
	}
	return &ScoreDB{file: file}, nil
}

// inserts only if the user has no result for that day; inserted=false means one exists
func (db *ScoreDB) Insert(r Row) (inserted bool, err error) {
	res, err := db.file.Exec(`INSERT INTO scores (guild_id, user_id, day, emojis) VALUES (?, ?, ?, ?)
		ON CONFLICT (guild_id, user_id, day) DO NOTHING;`, r.GuildID, r.UserID, r.Day, r.Emojis)
	if err != nil {
		return false, err
	}
	n, err := (*res).RowsAffected()
	return n > 0, err
}

// inserts or overwrites
func (db *ScoreDB) Upsert(r Row) error {
	_, err := db.file.Exec(`INSERT INTO scores (guild_id, user_id, day, emojis) VALUES (?, ?, ?, ?)
		ON CONFLICT (guild_id, user_id, day) DO UPDATE SET emojis = excluded.emojis;`, r.GuildID, r.UserID, r.Day, r.Emojis)
	return err
}

func (db *ScoreDB) Get(guild string, user string, day int) (row Row, ok bool, err error) {
	iter, err := db.file.Query(`SELECT emojis FROM scores WHERE guild_id = ? AND user_id = ? AND day = ?;`, guild, user, day)
	if err != nil {
		return Row{}, false, err
	}
	defer iter.Close() //nolint
	row = Row{GuildID: guild, UserID: user, Day: day}
	ok, err = iter.NextScan(&row.Emojis)
	return row, ok, err
}

// every result in a guild for a day
func (db *ScoreDB) Day(guild string, day int) ([]Row, error) {
	iter, err := db.file.Query(`SELECT user_id, emojis FROM scores WHERE guild_id = ? AND day = ?;`, guild, day)
	if err != nil {
		return nil, err
	}
	defer iter.Close() //nolint
	var rows []Row
	for {
		row := Row{GuildID: guild, Day: day}
		ok, err := iter.NextScan(&row.UserID, &row.Emojis)
		if err != nil {
			return nil, err
		}
		if !ok {
			return rows, nil
		}
		rows = append(rows, row)
	}
}

func (db *ScoreDB) Close() error {
	return db.file.Close()
}
