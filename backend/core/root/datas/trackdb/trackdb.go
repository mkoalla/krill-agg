package trackdb

import (
	"encoding/json"
	"fmt"
	"krill-agg/root/datas/sqlite"
	"strings"
	"time"
)

const FILE_TIMEOUT time.Duration = 5000 * time.Millisecond

// see scoredb; foreign keys are off per connection unless asked for, and the
// main channel's cascade depends on them
const PRAGMAS = "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"

// a guild's main channel must be one of its tracked channels: untracking it
// (or everything) clears the main channel with it
const SCHEMA = `
CREATE TABLE IF NOT EXISTS tracked (
	guild_id   TEXT NOT NULL,
	channel_id TEXT NOT NULL,
	PRIMARY KEY (guild_id, channel_id)
);
CREATE TABLE IF NOT EXISTS main_channel (
	guild_id   TEXT PRIMARY KEY,
	channel_id TEXT NOT NULL,
	FOREIGN KEY (guild_id, channel_id) REFERENCES tracked (guild_id, channel_id) ON DELETE CASCADE
);`

type TrackDB struct {
	file *sqlite.SQLiteDB
}

func New(path string) (*TrackDB, error) {
	file, err := sqlite.New(path+PRAGMAS, FILE_TIMEOUT)
	if err != nil {
		return nil, err
	}
	db := &TrackDB{file: file}
	if _, err := file.Exec(SCHEMA); err != nil {
		return nil, err
	}
	if err := db.migrateFileDict(); err != nil {
		return nil, fmt.Errorf("migrating old tracking data: %v", err)
	}
	return db, nil
}

func (db *TrackDB) Close() error { return db.file.Close() }

// tracking used to be a filedict (table data: guild -> json array of channels).
// moves those rows into tracked, in their stored order, and drops the old table
func (db *TrackDB) migrateFileDict() error {
	iter, err := db.file.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'data';`)
	if err != nil {
		return err
	}
	var name string
	found, err := iter.NextScan(&name)
	iter.Close() //nolint
	if err != nil || !found {
		return err
	}

	iter, err = db.file.Query(`SELECT key, value FROM data ORDER BY rowid;`)
	if err != nil {
		return err
	}
	var args []any
	for {
		var guild string
		var raw []byte
		ok, err := iter.NextScan(&guild, &raw)
		if err != nil {
			iter.Close() //nolint
			return err
		}
		if !ok {
			break
		}
		var channels []string
		if err := json.Unmarshal(raw, &channels); err != nil {
			iter.Close() //nolint
			return fmt.Errorf("guild %s: %v", guild, err)
		}
		for _, c := range channels {
			args = append(args, guild, c)
		}
	}
	iter.Close() //nolint

	return db.file.Tx(func(tx *sqlite.Tx) error {
		if len(args) > 0 {
			if err := tx.InsertRowsWith(`INSERT INTO tracked (guild_id, channel_id) VALUES`, `ON CONFLICT DO NOTHING`, 2, args); err != nil {
				return err
			}
		}
		_, err := tx.Exec(`DROP TABLE data;`)
		return err
	})
}

// every guild's tracked channels in the order they were added, and main channels
func (db *TrackDB) Load() (tracked map[string][]string, mains map[string]string, err error) {
	tracked = map[string][]string{}
	mains = map[string]string{}
	iter, err := db.file.Query(`SELECT guild_id, channel_id FROM tracked ORDER BY rowid;`)
	if err != nil {
		return nil, nil, err
	}
	for {
		var guild, channel string
		ok, err := iter.NextScan(&guild, &channel)
		if err != nil {
			iter.Close() //nolint
			return nil, nil, err
		}
		if !ok {
			break
		}
		tracked[guild] = append(tracked[guild], channel)
	}
	iter.Close() //nolint

	iter, err = db.file.Query(`SELECT guild_id, channel_id FROM main_channel;`)
	if err != nil {
		return nil, nil, err
	}
	defer iter.Close() //nolint
	for {
		var guild, channel string
		ok, err := iter.NextScan(&guild, &channel)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			break
		}
		mains[guild] = channel
	}
	return tracked, mains, nil
}

func trackTx(tx *sqlite.Tx, guild string, channels []string) error {
	if len(channels) == 0 {
		return nil
	}
	args := make([]any, 0, 2*len(channels))
	for _, c := range channels {
		args = append(args, guild, c)
	}
	return tx.InsertRowsWith(`INSERT INTO tracked (guild_id, channel_id) VALUES`, `ON CONFLICT DO NOTHING`, 2, args)
}

func setMainTx(tx *sqlite.Tx, guild, channel string) error {
	_, err := tx.Exec(`INSERT INTO main_channel (guild_id, channel_id) VALUES (?, ?)
		ON CONFLICT (guild_id) DO UPDATE SET channel_id = excluded.channel_id;`, guild, channel)
	return err
}

// tracks channels (already tracked ones are skipped) and, if main isn't empty,
// makes it the main channel. one transaction
func (db *TrackDB) Track(guild string, channels []string, main string) error {
	return db.file.Tx(func(tx *sqlite.Tx) error {
		if err := trackTx(tx, guild, channels); err != nil {
			return err
		}
		if main == "" {
			return nil
		}
		return setMainTx(tx, guild, main)
	})
}

// sets the main channel, tracking it first if it isn't
func (db *TrackDB) SetMain(guild, channel string) error {
	return db.Track(guild, []string{channel}, channel)
}

// untracks channels; the main channel goes with them if it's one
func (db *TrackDB) Untrack(guild string, channels []string) error {
	if len(channels) == 0 {
		return nil
	}
	args := make([]any, 0, 1+len(channels))
	args = append(args, guild)
	for _, c := range channels {
		args = append(args, c)
	}
	marks := strings.TrimSuffix(strings.Repeat("?, ", len(channels)), ", ")
	_, err := db.file.Exec(`DELETE FROM tracked WHERE guild_id = ? AND channel_id IN (`+marks+`);`, args...)
	return err
}

// untracks everything in the guild, main channel included
func (db *TrackDB) UntrackAll(guild string) error {
	_, err := db.file.Exec(`DELETE FROM tracked WHERE guild_id = ?;`, guild)
	return err
}

// the guild's main channel straight from the file, for checking the cascade
func (db *TrackDB) Main(guild string) (string, bool, error) {
	iter, err := db.file.Query(`SELECT channel_id FROM main_channel WHERE guild_id = ?;`, guild)
	if err != nil {
		return "", false, err
	}
	defer iter.Close() //nolint
	var channel string
	ok, err := iter.NextScan(&channel)
	return channel, ok, err
}
