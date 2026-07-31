package domain

const (
	Side      = 3
	BoardSize = Side * Side
)

type Board [BoardSize]Mark

// Cell returns mark that is located at that cell
func (b Board) Cell(i int) Mark {
	if i < 0 || i >= BoardSize {
		return Empty
	}

	return b[i]
}

// winnerInColumns scans columns for matches and returns Empty, X or O
func (b Board) winnerInColumns() Mark {
	for i := range Side {
		mark, mark1, mark2 := b[i], b[i+Side], b[Side*2+i]
		if mark == Empty {
			continue
		}

		if mark == mark1 && mark == mark2 {
			return mark
		}
	}

	return Empty
}

// winnerInDiagonals scans diagonals for matches and returns Empty, X or O
func (b Board) winnerInDiagonals() Mark {
	checkMarks := func(mark, mark1, mark2 Mark) Mark {
		if mark == Empty {
			return Empty
		}

		if mark == mark1 && mark == mark2 {
			return mark
		}

		return Empty
	}

	// From left-to-right
	if m := checkMarks(b[0], b[Side+1], b[Side*2+2]); m != Empty {
		return m
	}

	// From right-to-left
	return checkMarks(b[Side-1], b[Side+1], b[Side*2])
}

// winnerInRows scans rows for matches and returns Empty, X or O
func (b Board) winnerInRows() Mark {
	for i := 0; i < len(b); i += Side {
		mark, mark1, mark2 := b[i], b[i+1], b[i+2]
		if mark == Empty {
			continue
		}

		if mark == mark1 && mark == mark2 {
			return mark
		}
	}

	return Empty
}

// Winner returns XMark or OMark if there is a row match, Empty otherwise
func (b Board) Winner() Mark {
	if m := b.winnerInColumns(); m != Empty {
		return m
	}

	if m := b.winnerInRows(); m != Empty {
		return m
	}

	return b.winnerInDiagonals()
}

// Set adds a Mark at position specified, returns error ErrCellOutOfBounds, ErrCellOccupied
func (b *Board) Set(cell int, m Mark) error {
	if cell < 0 || cell >= BoardSize {
		return ErrCellOutOfBounds
	}

	if b[cell] != Empty {
		return ErrCellOccupied
	}

	b[cell] = m
	return nil
}

// IsFull returns true if table is full, false otherwise
func (b Board) IsFull() bool {
	for _, m := range b {
		if m == Empty {
			return false
		}
	}

	return true
}
