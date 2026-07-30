package usecase

import (
	"context"
	"slices"
	"sync"

	"github.com/kopetann/gotic_tac/internal/domain"
)

// Matchmaking is a FIFO queue: the two players who have waited longest are
// paired, and the one who waited longer takes X and so moves first.
type Matchmaking struct {
	mu      sync.Mutex
	queue   []domain.PlayerID
	matches Matches
}

func NewMatchmaking(matches Matches) *Matchmaking {
	return &Matchmaking{matches: matches}
}

func (m *Matchmaking) Join(ctx context.Context, p domain.PlayerID) error {
	if m.matches.InMatch(p) {
		return ErrAlreadyInMatch
	}

	m.mu.Lock()
	if slices.Contains(m.queue, p) {
		m.mu.Unlock()
		return ErrAlreadyQueued
	}

	m.queue = append(m.queue, p)
	if len(m.queue) < 2 {
		m.mu.Unlock()
		return nil
	}

	x, o := m.queue[0], m.queue[1]
	m.queue = slices.Delete(m.queue, 0, 2)
	m.mu.Unlock()

	// Start is called with the lock released: it notifies both clients, and
	// holding the queue lock across a network write would stall everyone else
	// trying to join.
	return m.matches.Start(ctx, x, o)
}

func (m *Matchmaking) Leave(p domain.PlayerID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if i := slices.Index(m.queue, p); i >= 0 {
		m.queue = slices.Delete(m.queue, i, i+1)
	}
}

func (m *Matchmaking) Waiting() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.queue)
}
