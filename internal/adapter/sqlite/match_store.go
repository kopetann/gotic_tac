package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kopetann/gotic_tac/internal/domain"
	"github.com/kopetann/gotic_tac/internal/usecase"
)

type MatchStore struct {
	db *sql.DB
}

func NewMatchStore(db *sql.DB) *MatchStore {
	return &MatchStore{db: db}
}

// Save upserts: Games calls this after every move, so the same match id is
// written once per move with a longer history each time.
func (s *MatchStore) Save(ctx context.Context, r usecase.MatchRecord) error {
	const query = `
		INSERT INTO matches (id, player_x, player_o, status, moves)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET status = excluded.status, moves = excluded.moves`

	_, err := s.db.ExecContext(ctx, query,
		string(r.ID), string(r.X), string(r.O), r.Status.String(), encodeMoves(r.Moves),
	)
	if err != nil {
		return fmt.Errorf("save match %s: %w", r.ID, err)
	}

	return nil
}

// ByID reads a match back. Nothing in the use case layer needs it, so it is not
// on the MatchStore port — it exists so the persistence claim is testable:
// a stored match must replay through domain.RestoreMatch into the same position.
func (s *MatchStore) ByID(ctx context.Context, id domain.MatchID) (usecase.MatchRecord, bool, error) {
	const query = `SELECT id, player_x, player_o, status, moves FROM matches WHERE id = ?`

	var (
		rec              usecase.MatchRecord
		rawID, x, o      string
		rawStatus, moves string
	)

	err := s.db.QueryRowContext(ctx, query, string(id)).
		Scan(&rawID, &x, &o, &rawStatus, &moves)
	if errors.Is(err, sql.ErrNoRows) {
		return usecase.MatchRecord{}, false, nil
	}

	if err != nil {
		return usecase.MatchRecord{}, false, fmt.Errorf("load match %s: %w", id, err)
	}

	status, err := parseStatus(rawStatus)
	if err != nil {
		return usecase.MatchRecord{}, false, fmt.Errorf("load match %s: %w", id, err)
	}

	decoded, err := decodeMoves(moves)
	if err != nil {
		return usecase.MatchRecord{}, false, fmt.Errorf("load match %s: %w", id, err)
	}

	rec = usecase.MatchRecord{
		ID:     domain.MatchID(rawID),
		X:      domain.PlayerID(x),
		O:      domain.PlayerID(o),
		Status: status,
		Moves:  decoded,
	}

	return rec, true, nil
}
