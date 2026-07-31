package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // pure-Go driver: no cgo, so the Docker image needs no toolchain
)

// Schema notes worth being able to defend:
//
//   - name_key holds the lowercased name and carries the UNIQUE index, so
//     "Alice" cannot register alongside "alice" while name still renders with
//     the capitalisation the player chose.
//   - moves is the full history in play order. Status is derivable by replaying
//     it through domain.RestoreMatch and is stored only so finished matches can
//     be queried without decoding every row.
//   - matches deliberately has no foreign key to players. A match is an
//     append-only record of something that happened, and adding the constraint
//     would make this store reject rows the in-memory store accepts — the two
//     must stay substitutable.
const schema = `
CREATE TABLE IF NOT EXISTS players (
	id            TEXT    PRIMARY KEY,
	name          TEXT    NOT NULL,
	name_key      TEXT    NOT NULL UNIQUE,
	password_hash TEXT    NOT NULL,
	wins          INTEGER NOT NULL DEFAULT 0,
	losses        INTEGER NOT NULL DEFAULT 0,
	draws         INTEGER NOT NULL DEFAULT 0
) STRICT;

CREATE TABLE IF NOT EXISTS matches (
	id       TEXT NOT NULL PRIMARY KEY,
	player_x TEXT NOT NULL,
	player_o TEXT NOT NULL,
	status   TEXT NOT NULL,
	moves    TEXT NOT NULL
) STRICT;

CREATE INDEX IF NOT EXISTS players_ranking ON players (wins DESC, losses ASC, name ASC);
`

// Open connects to the database at path and applies the schema. Passing
// ":memory:" gives an ephemeral database, which is what the tests use.
//
// WAL lets readers proceed during a write, and busy_timeout makes a blocked
// writer wait rather than fail instantly with SQLITE_BUSY.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)", path)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}

	// SQLite serialises writers anyway, so a single connection trades a little
	// read concurrency for the guarantee that we never see SQLITE_BUSY. At two
	// players per match this costs nothing measurable; a busier service would
	// raise this and rely on the busy_timeout above.
	db.SetMaxOpenConns(1)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite %q: %w", path, err)
	}

	if _, err := db.ExecContext(ctx, schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	return db, nil
}
