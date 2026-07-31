package sqlite

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kopetann/gotic_tac/internal/domain"
)

// encodeMoves turns records into readable strings like "X0, X3, O2"
func encodeMoves(moves []domain.Move) string {
	parts := make([]string, len(moves))
	for i, mv := range moves {
		parts[i] = mv.Mark.String() + strconv.Itoa(mv.Cell)
	}

	return strings.Join(parts, ",")
}

func decodeMoves(s string) ([]domain.Move, error) {
	if s == "" {
		return nil, nil
	}

	parts := strings.Split(s, ",")
	moves := make([]domain.Move, 0, len(parts))
	for _, part := range parts {
		if len(part) < 2 {
			return nil, fmt.Errorf("decode move %q: too short", part)
		}

		mark, err := parseMark(part[:1])
		if err != nil {
			return nil, err
		}

		cell, err := strconv.Atoi(part[1:])
		if err != nil {
			return nil, fmt.Errorf("decode move %q: %w", part, err)
		}

		moves = append(moves, domain.Move{Mark: mark, Cell: cell})
	}

	return moves, nil
}

func parseMark(s string) (domain.Mark, error) {
	switch s {
	case "X":
		return domain.X, nil
	case "O":
		return domain.O, nil
	default:
		return domain.Empty, fmt.Errorf("unknown mark %q", s)
	}
}

func parseStatus(s string) (domain.Status, error) {
	switch s {
	case "in_progress":
		return domain.StatusInProgress, nil

	case "x_won":
		return domain.StatusXWon, nil

	case "o_won":
		return domain.StatusOWon, nil

	case "draw":
		return domain.StatusDraw, nil

	default:
		return domain.StatusInProgress, fmt.Errorf("unknown status %q", s)
	}
}
