package domain

import (
	"strings"
	"testing"
)

// winningLines is written out by hand rather than derived from the board code,
// so it is an independent statement of the rules. If the implementation and the
// test shared one table, the test could only prove the code agrees with itself.
// match_test.go uses it too.
var winningLines = [8][3]int{
	{0, 1, 2}, {3, 4, 5}, {6, 7, 8}, // rows
	{0, 3, 6}, {1, 4, 7}, {2, 5, 8}, // columns
	{0, 4, 8}, {2, 4, 6}, // diagonals
}

// parseBoard builds a Board from a nine-cell layout using 'X', 'O' and '.' for
// an empty cell. Whitespace is skipped, so a case can be written on one line or
// laid out as a grid, whichever reads better.
func parseBoard(t *testing.T, layout string) Board {
	t.Helper()

	var (
		b Board
		i int
	)

	for _, r := range layout {
		var m Mark

		switch r {
		case 'X':
			m = X
		case 'O':
			m = O
		case '.':
			m = Empty
		case ' ', '\n', '\t':
			continue
		default:
			t.Fatalf("parseBoard(%q): unexpected character %q", layout, r)
		}

		if i >= BoardSize {
			t.Fatalf("parseBoard(%q): more than %d cells", layout, BoardSize)
		}

		b[i] = m
		i++
	}

	if i != BoardSize {
		t.Fatalf("parseBoard(%q): got %d cells, want %d", layout, i, BoardSize)
	}

	return b
}

// render draws a board as a grid for failure messages. It lives in the test
// file on purpose: presentation is not the domain's job, and the real renderer
// belongs to the CLI client.
func render(b Board) string {
	var sb strings.Builder

	for row := range Side {
		if row > 0 {
			sb.WriteByte('\n')
		}

		for col := range Side {
			switch b[row*Side+col] {
			case X:
				sb.WriteByte('X')
			case O:
				sb.WriteByte('O')
			default:
				sb.WriteByte('.')
			}
		}
	}

	return sb.String()
}

// Only X is written out here. TestWinnerIsSymmetric derives the O cases from
// the same table, so every position is covered for both players without the
// table being twice as long.
var winnerCases = []struct {
	name   string
	layout string
	want   Mark
}{
	{"empty board", ".........", Empty},

	{"row top", "XXX......", X},
	{"row middle", "...XXX...", X},
	{"row bottom", "......XXX", X},

	{"column left", "X..X..X..", X},
	{"column middle", ".X..X..X.", X},
	{"column right", "..X..X..X", X},

	// The top-left cell belongs to the other player here, so a column scan that
	// gives up on an empty first cell cannot slip through this one.
	{"column right with a busy corner", "O.X..X..X", X},

	{"diagonal top-left to bottom-right", "X...X...X", X},
	{"diagonal top-right to bottom-left", "..X.X.X..", X},

	{"two in a row is not a win", "XX.......", Empty},
	{"a line split between players", "XOX......", Empty},
	{"a diagonal blocked on the last cell", "X...X...O", Empty},
	{"both players two in a row", "XX.OO....", Empty},
	{"full board with no line", "XOXXOOOXX", Empty},
}

func TestWinner(t *testing.T) {
	for _, tt := range winnerCases {
		t.Run(tt.name, func(t *testing.T) {
			b := parseBoard(t, tt.layout)

			if got := b.Winner(); got != tt.want {
				t.Errorf("Winner() = %q, want %q\n%s", got, tt.want, render(b))
			}
		})
	}
}

// Nothing in the rules favours X, so swapping every mark must swap the result.
// This doubles the coverage of the table above and would catch a check that
// only ever compares against X.
func TestWinnerIsSymmetric(t *testing.T) {
	swap := func(layout string) string {
		return strings.Map(func(r rune) rune {
			switch r {
			case 'X':
				return 'O'
			case 'O':
				return 'X'
			default:
				return r
			}
		}, layout)
	}

	for _, tt := range winnerCases {
		t.Run(tt.name, func(t *testing.T) {
			b := parseBoard(t, swap(tt.layout))
			want := tt.want.Opponent() // the opponent of Empty is Empty

			if got := b.Winner(); got != want {
				t.Errorf("Winner() = %q, want %q\n%s", got, want, render(b))
			}
		})
	}
}

// Every line must be detected, so one the implementation forgets cannot hide
// behind the hand-picked cases above.
func TestWinnerDetectsEveryLine(t *testing.T) {
	for _, line := range winningLines {
		t.Run(lineName(line), func(t *testing.T) {
			var b Board
			for _, cell := range line {
				b[cell] = X
			}

			if got := b.Winner(); got != X {
				t.Errorf("Winner() = %q, want X for line %v\n%s", got, line, render(b))
			}
		})
	}
}

// The same lines, but with the opponent's marks scattered around them: a win
// has to be reported from the board alone, whatever else is on it.
func TestWinnerIgnoresUnrelatedMarks(t *testing.T) {
	for _, line := range winningLines {
		t.Run(lineName(line), func(t *testing.T) {
			onLine := map[int]bool{line[0]: true, line[1]: true, line[2]: true}

			var b Board
			for _, cell := range line {
				b[cell] = X
			}

			placed := 0
			for cell := range BoardSize {
				if placed == 2 {
					break
				}

				if !onLine[cell] {
					b[cell] = O
					placed++
				}
			}

			if got := b.Winner(); got != X {
				t.Errorf("Winner() = %q, want X for line %v\n%s", got, line, render(b))
			}
		})
	}
}

func TestIsFull(t *testing.T) {
	tests := []struct {
		name   string
		layout string
		want   bool
	}{
		{"empty", ".........", false},
		{"last cell empty", "XOXOXOXO.", false},
		{"first cell empty", ".OXOXOXOX", false},
		{"full", "XOXOXOXOX", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := parseBoard(t, tt.layout)

			if got := b.IsFull(); got != tt.want {
				t.Errorf("IsFull() = %v, want %v\n%s", got, tt.want, render(b))
			}
		})
	}
}

func TestCell(t *testing.T) {
	b := parseBoard(t, "XO.......")

	tests := []struct {
		name string
		cell int
		want Mark
	}{
		{"occupied by X", 0, X},
		{"occupied by O", 1, O},
		{"empty", 2, Empty},
		{"negative index", -1, Empty},
		{"past the end", BoardSize, Empty},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := b.Cell(tt.cell); got != tt.want {
				t.Errorf("Cell(%d) = %q, want %q", tt.cell, got, tt.want)
			}
		})
	}
}

func TestSet(t *testing.T) {
	tests := []struct {
		name    string
		layout  string
		cell    int
		wantErr error
	}{
		{"empty cell", ".........", 4, nil},
		{"occupied cell", "....X....", 4, ErrCellOccupied},
		{"negative index", ".........", -1, ErrCellOutOfBounds},
		{"past the end", ".........", BoardSize, ErrCellOutOfBounds},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := parseBoard(t, tt.layout)
			before := b

			err := b.Set(tt.cell, O)
			if err != tt.wantErr {
				t.Fatalf("Set(%d) error = %v, want %v", tt.cell, err, tt.wantErr)
			}

			// A rejected write must leave the board exactly as it was.
			if tt.wantErr != nil && b != before {
				t.Errorf("a rejected Set changed the board\n%s", render(b))
			}

			if tt.wantErr == nil && b[tt.cell] != O {
				t.Errorf("cell %d = %q, want O\n%s", tt.cell, b[tt.cell], render(b))
			}
		})
	}
}

func lineName(line [3]int) string {
	var sb strings.Builder

	for i, cell := range line {
		if i > 0 {
			sb.WriteByte('_')
		}

		sb.WriteByte(byte('0' + cell))
	}

	return sb.String()
}
