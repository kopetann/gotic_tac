package sqlite

import (
	"testing"

	"github.com/kopetann/gotic_tac/internal/domain"
)

func TestEncodeMovesRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		moves   []domain.Move
		encoded string
	}{
		{"empty", nil, ""},
		{"one move", []domain.Move{{Mark: domain.X, Cell: 4}}, "X4"},
		{
			"a full game",
			[]domain.Move{
				{Mark: domain.X, Cell: 0}, {Mark: domain.O, Cell: 3},
				{Mark: domain.X, Cell: 1}, {Mark: domain.O, Cell: 4},
				{Mark: domain.X, Cell: 2},
			},
			"X0,O3,X1,O4,X2",
		},
		{"last cell", []domain.Move{{Mark: domain.O, Cell: 8}}, "O8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := encodeMoves(tt.moves)
			if got != tt.encoded {
				t.Fatalf("encodeMoves = %q, want %q", got, tt.encoded)
			}

			decoded, err := decodeMoves(got)
			if err != nil {
				t.Fatalf("decodeMoves(%q): %v", got, err)
			}

			if len(decoded) != len(tt.moves) {
				t.Fatalf("decoded %d moves, want %d", len(decoded), len(tt.moves))
			}

			for i := range tt.moves {
				if decoded[i] != tt.moves[i] {
					t.Fatalf("move %d = %+v, want %+v", i, decoded[i], tt.moves[i])
				}
			}
		})
	}
}

// A corrupt row must surface as an error rather than decoding into a plausible
// but wrong game.
func TestDecodeMovesRejectsGarbage(t *testing.T) {
	tests := []string{
		"Q4",     // unknown mark
		"X",      // no cell
		"4",      // no mark
		"XX",     // cell is not a number
		"X0,,O3", // empty element
		"X0,Q1",  // valid prefix, bad suffix
	}

	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			if _, err := decodeMoves(in); err == nil {
				t.Fatalf("decodeMoves(%q) succeeded, want an error", in)
			}
		})
	}
}

func TestParseStatusRoundTrip(t *testing.T) {
	all := []domain.Status{
		domain.StatusInProgress,
		domain.StatusXWon,
		domain.StatusOWon,
		domain.StatusDraw,
	}

	for _, want := range all {
		t.Run(want.String(), func(t *testing.T) {
			got, err := parseStatus(want.String())
			if err != nil {
				t.Fatalf("parseStatus(%q): %v", want.String(), err)
			}

			if got != want {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}

	if _, err := parseStatus("nonsense"); err == nil {
		t.Fatal("parseStatus accepted an unknown status")
	}
}
