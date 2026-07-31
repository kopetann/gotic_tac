// Package storetest holds one behavioural contract that every store
// implementation must satisfy, whatever it is backed by.
//
// This is the Liskov substitution principle made executable: memory.PlayerStore
// and sqlite.PlayerStore run the identical suite, so cmd/server can pick either
// and no use case can tell the difference. A behaviour that only one of them
// honours fails here rather than in production.
package storetest

import (
	"context"
	"errors"
	"testing"

	"github.com/kopetann/gotic_tac/internal/domain"
	"github.com/kopetann/gotic_tac/internal/usecase"
)

// PlayerStore is the union of the three ports a player store satisfies. No
// production code depends on this combined interface — the use cases each
// depend on their own narrow slice of it.
type PlayerStore interface {
	Create(ctx context.Context, p *domain.Player) error
	ByName(ctx context.Context, name string) (*domain.Player, error)
	ByID(ctx context.Context, id domain.PlayerID) (*domain.Player, error)
	Save(ctx context.Context, p *domain.Player) error
	Top(ctx context.Context, limit int) ([]*domain.Player, error)
}

type MatchStore interface {
	Save(ctx context.Context, r usecase.MatchRecord) error
	ByID(ctx context.Context, id domain.MatchID) (usecase.MatchRecord, bool, error)
}

func newPlayer(t *testing.T, id, name string) *domain.Player {
	t.Helper()

	p, err := domain.NewPlayer(domain.PlayerID(id), name, "hash-of-"+name)
	if err != nil {
		t.Fatalf("NewPlayer(%q): %v", name, err)
	}

	return p
}

// RunPlayerStore exercises the PlayerStore contract. newStore must return an
// empty store; it is called once per subtest so cases cannot leak into
// each other.
func RunPlayerStore(t *testing.T, newStore func(t *testing.T) PlayerStore) {
	t.Helper()

	ctx := context.Background()

	t.Run("create then read back by id and name", func(t *testing.T) {
		s := newStore(t)
		want := newPlayer(t, "p1", "alice")
		if err := s.Create(ctx, want); err != nil {
			t.Fatalf("Create: %v", err)
		}

		for _, tc := range []struct {
			name string
			get  func() (*domain.Player, error)
		}{
			{"ByID", func() (*domain.Player, error) { return s.ByID(ctx, "p1") }},
			{"ByName", func() (*domain.Player, error) { return s.ByName(ctx, "alice") }},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got, err := tc.get()
				if err != nil {
					t.Fatalf("%s: %v", tc.name, err)
				}

				if got.ID != want.ID || got.Name != want.Name || got.PasswordHash != want.PasswordHash {
					t.Fatalf("got %+v, want %+v", got, want)
				}
			})
		}
	})

	t.Run("missing player is reported as not found", func(t *testing.T) {
		s := newStore(t)

		if _, err := s.ByID(ctx, "nobody"); !errors.Is(err, usecase.ErrPlayerNotFound) {
			t.Fatalf("ByID: got %v, want ErrPlayerNotFound", err)
		}

		if _, err := s.ByName(ctx, "nobody"); !errors.Is(err, usecase.ErrPlayerNotFound) {
			t.Fatalf("ByName: got %v, want ErrPlayerNotFound", err)
		}
	})

	t.Run("duplicate name is rejected", func(t *testing.T) {
		s := newStore(t)
		if err := s.Create(ctx, newPlayer(t, "p1", "alice")); err != nil {
			t.Fatalf("Create: %v", err)
		}

		if err := s.Create(ctx, newPlayer(t, "p2", "alice")); !errors.Is(err, usecase.ErrNameTaken) {
			t.Fatalf("got %v, want ErrNameTaken", err)
		}
	})

	// Uniqueness has to survive capitalisation, or two players appear on the
	// leaderboard under what a human reads as one name.
	t.Run("duplicate name is case insensitive", func(t *testing.T) {
		s := newStore(t)
		if err := s.Create(ctx, newPlayer(t, "p1", "alice")); err != nil {
			t.Fatalf("Create: %v", err)
		}

		if err := s.Create(ctx, newPlayer(t, "p2", "ALICE")); !errors.Is(err, usecase.ErrNameTaken) {
			t.Fatalf("got %v, want ErrNameTaken", err)
		}
	})

	t.Run("lookup by name ignores case", func(t *testing.T) {
		s := newStore(t)
		if err := s.Create(ctx, newPlayer(t, "p1", "Alice")); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := s.ByName(ctx, "aLiCe")
		if err != nil {
			t.Fatalf("ByName: %v", err)
		}

		// The stored spelling is preserved even though the lookup key is not.
		if got.Name != "Alice" {
			t.Fatalf("name = %q, want Alice", got.Name)
		}
	})

	t.Run("save persists stats", func(t *testing.T) {
		s := newStore(t)
		p := newPlayer(t, "p1", "alice")
		if err := s.Create(ctx, p); err != nil {
			t.Fatalf("Create: %v", err)
		}

		p.Record(domain.OutcomeWin)
		p.Record(domain.OutcomeDraw)
		if err := s.Save(ctx, p); err != nil {
			t.Fatalf("Save: %v", err)
		}

		got, err := s.ByID(ctx, "p1")
		if err != nil {
			t.Fatalf("ByID: %v", err)
		}

		if want := (domain.Stats{Wins: 1, Draws: 1}); got.Stats != want {
			t.Fatalf("stats = %+v, want %+v", got.Stats, want)
		}
	})

	t.Run("save of an unknown player is not a silent insert", func(t *testing.T) {
		s := newStore(t)
		if err := s.Save(ctx, newPlayer(t, "ghost", "ghost")); !errors.Is(err, usecase.ErrPlayerNotFound) {
			t.Fatalf("got %v, want ErrPlayerNotFound", err)
		}
	})

	// Callers mutate the players they are given. If a store hands out a pointer
	// into its own state, those mutations land without a Save and -race fires.
	t.Run("reads return an independent copy", func(t *testing.T) {
		s := newStore(t)
		if err := s.Create(ctx, newPlayer(t, "p1", "alice")); err != nil {
			t.Fatalf("Create: %v", err)
		}

		first, err := s.ByID(ctx, "p1")
		if err != nil {
			t.Fatalf("ByID: %v", err)
		}

		first.Stats.Wins = 99
		first.Name = "mallory"

		second, err := s.ByID(ctx, "p1")
		if err != nil {
			t.Fatalf("ByID: %v", err)
		}

		if second.Stats.Wins != 0 || second.Name != "alice" {
			t.Fatalf("mutating a returned player changed the store: %+v", second)
		}
	})

	t.Run("empty store has an empty leaderboard", func(t *testing.T) {
		s := newStore(t)
		got, err := s.Top(ctx, 10)
		if err != nil {
			t.Fatalf("Top: %v", err)
		}

		if len(got) != 0 {
			t.Fatalf("got %d players, want 0", len(got))
		}
	})

	t.Run("top orders by wins desc then losses asc", func(t *testing.T) {
		s := newStore(t)
		seed := []struct {
			id, name string
			stats    domain.Stats
		}{
			{"p1", "carol", domain.Stats{Wins: 5, Losses: 4}},
			{"p2", "alice", domain.Stats{Wins: 5, Losses: 1}},
			{"p3", "bob", domain.Stats{Wins: 9, Losses: 0}},
			{"p4", "dave", domain.Stats{Wins: 0, Losses: 2}},
		}

		for _, sp := range seed {
			p := newPlayer(t, sp.id, sp.name)
			p.Stats = sp.stats
			if err := s.Create(ctx, p); err != nil {
				t.Fatalf("Create %s: %v", sp.name, err)
			}

			if err := s.Save(ctx, p); err != nil {
				t.Fatalf("Save %s: %v", sp.name, err)
			}
		}

		got, err := s.Top(ctx, 10)
		if err != nil {
			t.Fatalf("Top: %v", err)
		}

		want := []string{"bob", "alice", "carol", "dave"}
		if len(got) != len(want) {
			t.Fatalf("got %d players, want %d", len(got), len(want))
		}

		for i, name := range want {
			if got[i].Name != name {
				t.Fatalf("position %d = %q, want %q", i, got[i].Name, name)
			}
		}
	})

	t.Run("top respects the limit", func(t *testing.T) {
		s := newStore(t)
		for _, name := range []string{"alice", "bob", "carol"} {
			if err := s.Create(ctx, newPlayer(t, "id-"+name, name)); err != nil {
				t.Fatalf("Create %s: %v", name, err)
			}
		}

		got, err := s.Top(ctx, 2)
		if err != nil {
			t.Fatalf("Top: %v", err)
		}

		if len(got) != 2 {
			t.Fatalf("got %d players, want 2", len(got))
		}
	})
}

// RunMatchStore exercises the MatchStore contract.
func RunMatchStore(t *testing.T, newStore func(t *testing.T) MatchStore) {
	t.Helper()

	ctx := context.Background()

	record := func(status domain.Status, moves ...domain.Move) usecase.MatchRecord {
		return usecase.MatchRecord{ID: "m1", X: "p1", O: "p2", Status: status, Moves: moves}
	}

	t.Run("save then read back", func(t *testing.T) {
		s := newStore(t)
		want := record(domain.StatusInProgress,
			domain.Move{Mark: domain.X, Cell: 4},
			domain.Move{Mark: domain.O, Cell: 0},
		)

		if err := s.Save(ctx, want); err != nil {
			t.Fatalf("Save: %v", err)
		}

		got, ok, err := s.ByID(ctx, "m1")
		if err != nil {
			t.Fatalf("ByID: %v", err)
		}

		if !ok {
			t.Fatal("match not found after Save")
		}

		if got.ID != want.ID || got.X != want.X || got.O != want.O || got.Status != want.Status {
			t.Fatalf("got %+v, want %+v", got, want)
		}

		if len(got.Moves) != len(want.Moves) {
			t.Fatalf("got %d moves, want %d", len(got.Moves), len(want.Moves))
		}

		for i := range want.Moves {
			if got.Moves[i] != want.Moves[i] {
				t.Fatalf("move %d = %+v, want %+v", i, got.Moves[i], want.Moves[i])
			}
		}
	})

	t.Run("missing match is reported as absent, not an error", func(t *testing.T) {
		s := newStore(t)
		_, ok, err := s.ByID(ctx, "nope")
		if err != nil {
			t.Fatalf("ByID: %v", err)
		}

		if ok {
			t.Fatal("an empty store reported a match")
		}
	})

	// Games saves the same match after every move, so overwriting has to work.
	t.Run("save upserts", func(t *testing.T) {
		s := newStore(t)
		if err := s.Save(ctx, record(domain.StatusInProgress, domain.Move{Mark: domain.X, Cell: 0})); err != nil {
			t.Fatalf("first Save: %v", err)
		}

		final := record(domain.StatusXWon,
			domain.Move{Mark: domain.X, Cell: 0},
			domain.Move{Mark: domain.O, Cell: 3},
			domain.Move{Mark: domain.X, Cell: 1},
			domain.Move{Mark: domain.O, Cell: 4},
			domain.Move{Mark: domain.X, Cell: 2},
		)
		if err := s.Save(ctx, final); err != nil {
			t.Fatalf("second Save: %v", err)
		}

		got, ok, err := s.ByID(ctx, "m1")
		if err != nil || !ok {
			t.Fatalf("ByID: %v (found=%v)", err, ok)
		}

		if got.Status != domain.StatusXWon {
			t.Fatalf("status = %v, want x_won", got.Status)
		}

		if len(got.Moves) != 5 {
			t.Fatalf("got %d moves, want 5", len(got.Moves))
		}
	})

	t.Run("a match with no moves round-trips", func(t *testing.T) {
		s := newStore(t)
		if err := s.Save(ctx, record(domain.StatusInProgress)); err != nil {
			t.Fatalf("Save: %v", err)
		}

		got, ok, err := s.ByID(ctx, "m1")
		if err != nil || !ok {
			t.Fatalf("ByID: %v (found=%v)", err, ok)
		}

		if len(got.Moves) != 0 {
			t.Fatalf("got %d moves, want 0", len(got.Moves))
		}
	})

	// The point of storing history: a persisted match must replay into the same
	// position through the domain's own rules. This is what "survives a restart"
	// actually means.
	t.Run("a stored match replays into the same position", func(t *testing.T) {
		s := newStore(t)
		final := record(domain.StatusXWon,
			domain.Move{Mark: domain.X, Cell: 0},
			domain.Move{Mark: domain.O, Cell: 3},
			domain.Move{Mark: domain.X, Cell: 1},
			domain.Move{Mark: domain.O, Cell: 4},
			domain.Move{Mark: domain.X, Cell: 2},
		)

		if err := s.Save(ctx, final); err != nil {
			t.Fatalf("Save: %v", err)
		}

		got, ok, err := s.ByID(ctx, "m1")
		if err != nil || !ok {
			t.Fatalf("ByID: %v (found=%v)", err, ok)
		}

		restored, err := domain.RestoreMatch(got.ID, got.X, got.O, got.Moves)
		if err != nil {
			t.Fatalf("RestoreMatch: %v", err)
		}

		if restored.Status() != domain.StatusXWon {
			t.Fatalf("replayed status = %v, want x_won", restored.Status())
		}

		if winner, ok := restored.Winner(); !ok || winner != "p1" {
			t.Fatalf("replayed winner = %q (%v), want p1", winner, ok)
		}
	})
}
