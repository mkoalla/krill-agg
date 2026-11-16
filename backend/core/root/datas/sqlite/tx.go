package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// sqlite's default cap on bound parameters per statement
const MAX_VARIABLES = 32766

type Tx struct {
	tx  *sql.Tx
	ctx context.Context
}

// runs f in a transaction: committed if f returns nil, rolled back otherwise
func (db *SQLiteDB) Tx(f func(tx *Tx) error) error {
	if !db.IsOpen() {
		return errors.New("connection closed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), db.timeout)
	defer cancel()

	tx, err := db.instance.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := f(&Tx{tx: tx, ctx: ctx}); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}

func (tx *Tx) Exec(query string, args ...any) (sql.Result, error) {
	return tx.tx.ExecContext(tx.ctx, query, args...)
}

// multi-row insert: head is everything before the rows (e.g. "REPLACE INTO t (a, b) VALUES"),
// args is width values per row, flattened. split into as few statements as the
// parameter cap allows; run inside a Tx so the batch stays all-or-nothing
func (tx *Tx) InsertRows(head string, width int, args []any) error {
	return tx.InsertRowsWith(head, "", width, args)
}

// InsertRows with a tail after the rows, e.g. an ON CONFLICT clause
func (tx *Tx) InsertRowsWith(head string, tail string, width int, args []any) error {
	if width <= 0 || len(args)%width != 0 {
		return errors.New("InsertRows: args aren't a whole number of rows")
	}
	row := "(" + strings.TrimSuffix(strings.Repeat("?, ", width), ", ") + ")"
	perStatement := (MAX_VARIABLES / width) * width
	for start := 0; start < len(args); start += perStatement {
		chunk := args[start:min(start+perStatement, len(args))]
		rows := strings.TrimSuffix(strings.Repeat(row+", ", len(chunk)/width), ", ")
		if _, err := tx.Exec(head+" "+rows+" "+tail+";", chunk...); err != nil {
			return err
		}
	}
	return nil
}
