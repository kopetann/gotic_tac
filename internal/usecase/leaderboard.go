package usecase

import "context"

const (
	DefaultLeaderboardLimit = 10
	MaxLeaderboardLimit     = 100
)

type Standing struct {
	Rank   int
	Name   string
	Wins   int
	Losses int
	Draws  int
}

type Leaderboard struct {
	players StandingsReader
}

func NewLeaderboard(players StandingsReader) *Leaderboard {
	return &Leaderboard{players: players}
}

// Top clamps rather than rejects: a nonsensical limit from a client is not
// worth failing a read-only request over.
func (l *Leaderboard) Top(ctx context.Context, limit int) ([]Standing, error) {
	if limit <= 0 {
		limit = DefaultLeaderboardLimit
	}

	if limit > MaxLeaderboardLimit {
		limit = MaxLeaderboardLimit
	}

	players, err := l.players.Top(ctx, limit)
	if err != nil {
		return nil, err
	}

	// Ordering is the store's job and part of the port contract; ranking the
	// ordered result is ours.
	standings := make([]Standing, len(players))
	for i, p := range players {
		standings[i] = Standing{
			Rank:   i + 1,
			Name:   p.Name,
			Wins:   p.Stats.Wins,
			Losses: p.Stats.Losses,
			Draws:  p.Stats.Draws,
		}
	}

	return standings, nil
}
