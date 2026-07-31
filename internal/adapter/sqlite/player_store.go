package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/kopetann/gotic_tac/internal/domain"
	"github.com/kopetann/gotic_tac/internal/usecase"
	sqlitedriver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// PlayerStore is the durable twin of memory.PlayerStore and satisfies the same
// three ports. Both run the conformance suite in adapter/storetest, which is
// what makes swapping them in cmd/server a one-line change.
type PlayerStore struct {
	db *sql.DB
}

func NewPlayerStore(db *sql.DB) *PlayerStore {
	return &PlayerStore{db: db}
}

func (s *PlayerStore) Create(ctx context.Context, p *domain.Player) error {
	const query = `
		INSERT INTO players (id, name, name_key, password_hash, wins, losses, draws)
		VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.ExecContext(ctx, query,
		string(p.ID), p.Name, nameKey(p.Name), p.PasswordHash,
		p.Stats.Wins, p.Stats.Losses, p.Stats.Draws,
	)
	if err != nil {
		// The UNIQUE index is the real guard against a duplicate name: checking
		// first and inserting after would leave a window for two concurrent
		// registrations to both pass the check.
		if isUniqueViolation(err) {
			return usecase.ErrNameTaken
		}

		return fmt.Errorf("create player: %w", err)
	}

	return nil
}

func (s *PlayerStore) ByName(ctx context.Context, name string) (*domain.Player, error) {
	const query = `
		SELECT id, name, password_hash, wins, losses, draws
		FROM players WHERE name_key = ?`

	return s.queryOne(ctx, query, nameKey(name))
}

func (s *PlayerStore) ByID(ctx context.Context, id domain.PlayerID) (*domain.Player, error) {
	const query = `
		SELECT id, name, password_hash, wins, losses, draws
		FROM players WHERE id = ?`

	return s.queryOne(ctx, query, string(id))
}

func (s *PlayerStore) Save(ctx context.Context, p *domain.Player) error {
	const query = `
		UPDATE players
		SET name = ?, name_key = ?, password_hash = ?, wins = ?, losses = ?, draws = ?
		WHERE id = ?`

	res, err := s.db.ExecContext(ctx, query,
		p.Name, nameKey(p.Name), p.PasswordHash,
		p.Stats.Wins, p.Stats.Losses, p.Stats.Draws, string(p.ID),
	)
	if err != nil {
		return fmt.Errorf("save player: %w", err)
	}

	// An UPDATE that matches nothing is not an error in SQL, so a missing
	// player has to be detected here or a lost row would look like a success.
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("save player: %w", err)
	}

	if n == 0 {
		return usecase.ErrPlayerNotFound
	}

	return nil
}

func (s *PlayerStore) Top(ctx context.Context, limit int) ([]*domain.Player, error) {
	// The ORDER BY is the port's contract, not an implementation detail: the
	// in-memory store sorts by exactly the same keys, name last so the order is
	// deterministic when records tie.
	const query = `
		SELECT id, name, password_hash, wins, losses, draws
		FROM players
		ORDER BY wins DESC, losses ASC, name ASC
		LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("leaderboard: %w", err)
	}
	defer rows.Close()

	var players []*domain.Player
	for rows.Next() {
		p, err := scanPlayer(rows)
		if err != nil {
			return nil, fmt.Errorf("leaderboard: %w", err)
		}

		players = append(players, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("leaderboard: %w", err)
	}

	return players, nil
}

func (s *PlayerStore) queryOne(ctx context.Context, query string, arg any) (*domain.Player, error) {
	p, err := scanPlayer(s.db.QueryRowContext(ctx, query, arg))
	if errors.Is(err, sql.ErrNoRows) {
		// Translated at the boundary so use cases never import database/sql.
		return nil, usecase.ErrPlayerNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("load player: %w", err)
	}

	return p, nil
}

// scanner covers both *sql.Row and *sql.Rows so one scan function serves the
// single-row and multi-row queries.
type scanner interface {
	Scan(dest ...any) error
}

func scanPlayer(sc scanner) (*domain.Player, error) {
	var (
		p  domain.Player
		id string
	)

	err := sc.Scan(&id, &p.Name, &p.PasswordHash, &p.Stats.Wins, &p.Stats.Losses, &p.Stats.Draws)
	if err != nil {
		return nil, err
	}

	p.ID = domain.PlayerID(id)
	return &p, nil
}

func nameKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func isUniqueViolation(err error) bool {
	var sqlErr *sqlitedriver.Error
	if !errors.As(err, &sqlErr) {
		return false
	}

	return sqlErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE ||
		sqlErr.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY
}
