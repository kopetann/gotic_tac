package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/kopetann/gotic_tac/internal/adapter/sqlite"
	"github.com/kopetann/gotic_tac/internal/adapter/storetest"
	"github.com/kopetann/gotic_tac/internal/domain"
)

// newDB opens a database on a real file under t.TempDir rather than ":memory:",
// so the tests exercise the same journal and locking behaviour the server gets
// in Docker. Go removes the directory when the test finishes.
func newDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	t.Cleanup(func() { db.Close() })
	return db
}

func TestPlayerStore(t *testing.T) {
	storetest.RunPlayerStore(t, func(t *testing.T) storetest.PlayerStore {
		return sqlite.NewPlayerStore(newDB(t))
	})
}

func TestMatchStore(t *testing.T) {
	storetest.RunMatchStore(t, func(t *testing.T) storetest.MatchStore {
		return sqlite.NewMatchStore(newDB(t))
	})
}

// Reopening must apply the schema without complaining and without losing data,
// which is exactly what a server restart does.
func TestSchemaSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "reopen.db")

	alice, err := domain.NewPlayer("p1", "alice", "hash")
	if err != nil {
		t.Fatalf("NewPlayer: %v", err)
	}

	first, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}

	if err := sqlite.NewPlayerStore(first).Create(ctx, alice); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer second.Close()

	got, err := sqlite.NewPlayerStore(second).ByName(ctx, "alice")
	if err != nil {
		t.Fatalf("ByName after reopen: %v", err)
	}

	if got.Name != "alice" {
		t.Fatalf("name = %q, want alice", got.Name)
	}
}
