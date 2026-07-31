package usecase

import (
	"context"

	"github.com/kopetann/gotic_tac/internal/domain"
)

type PlayerRegistry interface {
	Create(ctx context.Context, p *domain.Player) error
	ByName(ctx context.Context, name string) (*domain.Player, error)
}

type PlayerRecorder interface {
	ByID(ctx context.Context, id domain.PlayerID) (*domain.Player, error)
	Save(ctx context.Context, p *domain.Player) error
}

// Top returns players ordered by wins descending, then losses ascending.
// Ordering is part of the contract so the ranking shown to users does not
// depend on which implementation is wired in.
type StandingsReader interface {
	Top(ctx context.Context, limit int) ([]*domain.Player, error)
}

// MatchRecord is an immutable snapshot of a match, built while the Games mutex
// is held. Handing *domain.Match to a store instead would let the store read
// the board while another goroutine is mutating it.
type MatchRecord struct {
	ID     domain.MatchID
	X      domain.PlayerID
	O      domain.PlayerID
	Status domain.Status
	Moves  []domain.Move
}

type MatchStore interface {
	Save(ctx context.Context, r MatchRecord) error
}

type Hasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) error
}

type TokenIssuer interface {
	Issue(id domain.PlayerID) (string, error)
	Verify(token string) (domain.PlayerID, error)
}

type IDGenerator interface {
	NewID() string
}

// Notify must not block: it is called while a player's move is being served,
// and a slow or dead peer must not stall the other player's game.
type Notifier interface {
	Notify(id domain.PlayerID, ev Event)
}

// Matches is what matchmaking needs from the game service. Naming the two
// methods it actually calls keeps Matchmaking testable without a real Games.
type Matches interface {
	Start(ctx context.Context, x, o domain.PlayerID) error
	InMatch(p domain.PlayerID) bool
}
