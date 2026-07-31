package memory

import (
	"context"
	"sync"

	"github.com/kopetann/gotic_tac/internal/domain"
	"github.com/kopetann/gotic_tac/internal/usecase"
)

type MatchStore struct {
	mu      sync.RWMutex
	records map[domain.MatchID]usecase.MatchRecord
}

func NewMatchStore() *MatchStore {
	return &MatchStore{records: make(map[domain.MatchID]usecase.MatchRecord)}
}

func (s *MatchStore) Save(_ context.Context, r usecase.MatchRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// The move slice belongs to the caller; copying it keeps a later append
	// from rewriting a record we have already stored.
	stored := r
	stored.Moves = make([]domain.Move, len(r.Moves))
	copy(stored.Moves, r.Moves)

	s.records[r.ID] = stored
	return nil
}

func (s *MatchStore) ByID(_ context.Context, id domain.MatchID) (usecase.MatchRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.records[id]
	return r, ok, nil
}
