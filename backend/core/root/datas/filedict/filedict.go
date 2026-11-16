package filedict

import (
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"krill-agg/root/datas/sqlite"
	"sync"
	"sync/atomic"
	"time"
)

type FileDict[K comparable, V any] struct {
	data   map[K]V
	path   string
	file   *sqlite.SQLiteDB
	mutex  sync.RWMutex
	closed *atomic.Bool
}

var ErrIsClosed = errors.New("the FileDict is closed")

const FILE_TIMEOUT time.Duration = 5000 * time.Millisecond

func newDBWithSchema(path string) (*sqlite.SQLiteDB, error) {
	db, err := sqlite.New(path, FILE_TIMEOUT)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS data (
		key PRIMARY KEY,
		value BLOB NOT NULL
	);`)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func New[K comparable, V any](path string, validate func(*K, *V) bool) (*FileDict[K, V], error) {
	table := FileDict[K, V]{
		data:   make(map[K]V),
		path:   path,
		closed: &atomic.Bool{},
	}
	file, err := newDBWithSchema(path)
	if err != nil {
		return nil, err
	}
	table.file = file
	if err := table.SyncRead(); err != nil {
		return nil, err
	}
	for k, v := range table.data {
		if !validate(&k, &v) {
			return nil, fmt.Errorf("invalidated entry: %v | %v", k, v)
		}
	}
	return &table, nil
}
func (table *FileDict[K, V]) Iter() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for key, value := range table.data {
			if !yield(key, value) {
				return
			}
		}
	}
}

// write the whole map to the file, replacing what's there. one transaction,
// so a failure part way leaves the file as it was
func (table *FileDict[K, V]) SyncWrite() error {
	if err := table.lockOrClosed(); err != nil {
		return err
	}
	defer table.mutex.Unlock()
	return table.syncWriteLocked()
}

// caller holds the write lock
func (table *FileDict[K, V]) syncWriteLocked() error {
	args := make([]any, 0, len(table.data)*2)
	for k, v := range table.data {
		jsonData, err := json.Marshal(&v)
		if err != nil {
			return fmt.Errorf("while marshaling %v: %v", v, err)
		}
		args = append(args, k, jsonData)
	}
	return table.file.Tx(func(tx *sqlite.Tx) error {
		if _, err := tx.Exec(`DELETE FROM data;`); err != nil {
			return err
		}
		return tx.InsertRows("REPLACE INTO data (key, value) VALUES", 2, args)
	})
}

// partial writes: change one key in memory and on file together.
// memory is only touched once the file write succeeds
func (table *FileDict[K, V]) SetSync(key K, value V) error {
	if err := table.lockOrClosed(); err != nil {
		return err
	}
	defer table.mutex.Unlock()
	jsonData, err := json.Marshal(&value)
	if err != nil {
		return fmt.Errorf("while marshaling %v: %v", value, err)
	}
	if _, err := table.file.Exec(`REPLACE INTO data (key, value) VALUES (?, ?);`, key, jsonData); err != nil {
		return err
	}
	table.data[key] = value
	return nil
}
func (table *FileDict[K, V]) DropSync(key K) error {
	if err := table.lockOrClosed(); err != nil {
		return err
	}
	defer table.mutex.Unlock()
	if _, err := table.file.Exec(`DELETE FROM data WHERE key = ?;`, key); err != nil {
		return err
	}
	delete(table.data, key)
	return nil
}

// load contents from file
func (table *FileDict[K, V]) SyncRead() error {
	if err := table.lockOrClosed(); err != nil {
		return err
	}
	defer table.mutex.Unlock()

	iter, err := table.file.Query(`SELECT key, value FROM data;`)
	if err != nil {
		return err
	}
	defer iter.Close() //nolint

	table.data = make(map[K]V)
	for {
		var key K
		var raw []byte
		ok, err := iter.NextScan(&key, &raw)
		if err != nil {
			return err
		}
		if !ok {
			break
		}
		var value V
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		table.data[key] = value
	}
	return nil
}

func (table *FileDict[K, V]) IsClosed() bool {
	return table.closed.Load()
}

func (table *FileDict[K, V]) lockOrClosed() error {
	if table.IsClosed() {
		return ErrIsClosed
	}
	table.mutex.Lock()
	if table.IsClosed() {
		table.mutex.Unlock()
		return ErrIsClosed
	}
	return nil
}

// flushes anything changed with Set/Drop, then permanent read-only state afterwards.
// closes even if the flush fails, reporting both
func (table *FileDict[K, V]) Close() error {
	if err := table.lockOrClosed(); err != nil {
		return err
	}
	defer table.mutex.Unlock()
	errWrite := table.syncWriteLocked()
	table.closed.Store(true)
	return errors.Join(errWrite, table.file.Close())
}
func (table *FileDict[K, V]) Get(key K) (value V, ok bool) {
	table.mutex.RLock()
	defer table.mutex.RUnlock()
	//
	value, ok = table.data[key]
	return
}

// memory only; persisted by the next SyncWrite or Close. see SetSync
func (table *FileDict[K, V]) Set(key K, value V) error {
	if err := table.lockOrClosed(); err != nil {
		return err
	}
	defer table.mutex.Unlock()
	//
	table.data[key] = value
	return nil
}

// memory only; persisted by the next SyncWrite or Close. see DropSync
func (table *FileDict[K, V]) Drop(key K) error {
	if err := table.lockOrClosed(); err != nil {
		return err
	}
	defer table.mutex.Unlock()
	//
	if _, ok := table.data[key]; !ok {
		return nil
	}
	delete(table.data, key)
	return nil
}
