package memory

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/kopetann/gotic_tac/internal/domain"
	"github.com/kopetann/gotic_tac/internal/usecase"
)

// PlayerStore satisfies usecase.PlayerRegistry, usecase.PlayerRecorder and
// usecase.StandingsReader at once. It is the substitute the SQLite store is
// checked against: both run the same conformance suite, so either can be wired
// in without a use case noticing.
type PlayerStore struct {
	mu     sync.RWMutex
	byID   map[domain.PlayerID]domain.Player
	byName map[string]domain.PlayerID
}

func NewPlayerStore() *PlayerStore {
	return &PlayerStore{
		byID:   make(map[domain.PlayerID]domain.Player),
		byName: make(map[string]domain.PlayerID),
	}
}

func (s *PlayerStore) Create(_ context.Context, p *domain.Player) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := nameKey(p.Name)
	if _, taken := s.byName[key]; taken {
		return usecase.ErrNameTaken
	}

	s.byID[p.ID] = *p
	s.byName[key] = p.ID
	return nil
}

func (s *PlayerStore) ByName(_ context.Context, name string) (*domain.Player, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.byName[nameKey(name)]
	if !ok {
		return nil, usecase.ErrPlayerNotFound
	}

	return s.copyOf(id)
}

func (s *PlayerStore) ByID(_ context.Context, id domain.PlayerID) (*domain.Player, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.copyOf(id)
}

func (s *PlayerStore) Save(_ context.Context, p *domain.Player) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.byID[p.ID]; !ok {
		return usecase.ErrPlayerNotFound
	}

	s.byID[p.ID] = *p
	return nil
}

func (s *PlayerStore) Top(_ context.Context, limit int) ([]*domain.Player, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	all := make([]*domain.Player, 0, len(s.byID))
	for _, p := range s.byID {
		copied := p
		all = append(all, &copied)
	}

	// Name is the final tiebreak only so the order is deterministic; without it
	// two equally ranked players swap places between calls and the leaderboard
	// test flaps.
	sort.Slice(all, func(i, j int) bool {
		a, b := all[i], all[j]
		switch {
		case a.Stats.Wins != b.Stats.Wins:
			return a.Stats.Wins > b.Stats.Wins
		case a.Stats.Losses != b.Stats.Losses:
			return a.Stats.Losses < b.Stats.Losses
		default:
			return a.Name < b.Name
		}
	})

	if len(all) > limit {
		all = all[:limit]
	}

	return all, nil
}

// copyOf assumes the read lock is held. Callers get their own Player, so a
// caller recording stats cannot reach back into the store.
func (s *PlayerStore) copyOf(id domain.PlayerID) (*domain.Player, error) {
	p, ok := s.byID[id]
	if !ok {
		return nil, usecase.ErrPlayerNotFound
	}

	return &p, nil
}

// Names are unique case-insensitively, so "Alice" cannot register alongside
// "alice" and impersonate them on the leaderboard.
func nameKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
