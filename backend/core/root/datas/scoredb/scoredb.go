package scoredb

import (
	"krill-agg/root/datas/sqlite"
	"strings"
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

// inserts or overwrites many rows in one transaction
func (db *ScoreDB) UpsertMany(rows []Row) error {
	args := make([]any, 0, len(rows)*4)
	for _, r := range rows {
		args = append(args, r.GuildID, r.UserID, r.Day, r.Emojis)
	}
	return db.file.Tx(func(tx *sqlite.Tx) error {
		return tx.InsertRowsWith(`INSERT INTO scores (guild_id, user_id, day, emojis) VALUES`,
			`ON CONFLICT (guild_id, user_id, day) DO UPDATE SET emojis = excluded.emojis`, 4, args)
	})
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

// " AND user_id NOT IN (?, ...)" and its args, or nothing to exclude
func notIn(exclude []string) (string, []any) {
	if len(exclude) == 0 {
		return "", nil
	}
	args := make([]any, len(exclude))
	for j, id := range exclude {
		args[j] = id
	}
	return " AND user_id NOT IN (" + strings.TrimSuffix(strings.Repeat("?, ", len(exclude)), ", ") + ")", args
}

// every result in a guild from krillion #minDay on, except the excluded
// users' (the leaderboards' source)
func (db *ScoreDB) Guild(guild string, exclude []string, minDay int) ([]Row, error) {
	clause, args := notIn(exclude)
	iter, err := db.file.Query(`SELECT user_id, day, emojis FROM scores WHERE guild_id = ? AND day >= ?`+clause+`;`, append([]any{guild, minDay}, args...)...)
	if err != nil {
		return nil, err
	}
	defer iter.Close() //nolint
	var rows []Row
	for {
		row := Row{GuildID: guild}
		ok, err := iter.NextScan(&row.UserID, &row.Day, &row.Emojis)
		if err != nil {
			return nil, err
		}
		if !ok {
			return rows, nil
		}
		rows = append(rows, row)
	}
}

// everyone with a result in the guild from krillion #minDay on, except the excluded
func (db *ScoreDB) Players(guild string, exclude []string, minDay int) ([]string, error) {
	clause, args := notIn(exclude)
	iter, err := db.file.Query(`SELECT DISTINCT user_id FROM scores WHERE guild_id = ? AND day >= ?`+clause+`;`, append([]any{guild, minDay}, args...)...)
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

// how many results a user has in a guild
func (db *ScoreDB) CountUser(guild, user string) (int, error) {
	iter, err := db.file.Query(`SELECT COUNT(*) FROM scores WHERE guild_id = ? AND user_id = ?;`, guild, user)
	if err != nil {
		return 0, err
	}
	defer iter.Close() //nolint
	var n int
	_, err = iter.NextScan(&n)
	return n, err
}

// removes a user's result for a day; deleted=false means there wasn't one
func (db *ScoreDB) Delete(guild, user string, day int) (deleted bool, err error) {
	res, err := db.file.Exec(`DELETE FROM scores WHERE guild_id = ? AND user_id = ? AND day = ?;`, guild, user, day)
	if err != nil {
		return false, err
	}
	n, err := (*res).RowsAffected()
	return n > 0, err
}

// removes every result a user has in a guild; n is how many there were
func (db *ScoreDB) DeleteUser(guild, user string) (n int, err error) {
	res, err := db.file.Exec(`DELETE FROM scores WHERE guild_id = ? AND user_id = ?;`, guild, user)
	if err != nil {
		return 0, err
	}
	rows, err := (*res).RowsAffected()
	return int(rows), err
}

func (db *ScoreDB) Close() error {
	return db.file.Close()
}
