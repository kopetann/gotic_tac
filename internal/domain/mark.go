package domain

type Mark uint8

const (
	Empty Mark = iota
	X
	O
)

func (m Mark) String() string {
	switch m {
	case X:
		return "X"
	case O:
		return "O"
	default:
		return " "
	}
}

/*
Opponent returns the mark playing against m. The opponent of Empty is Empty,
which keeps the function total and avoids an error return for a case that
callers never legitimately hit.
*/
func (m Mark) Opponent() Mark {
	switch m {
	case X:
		return O
	case O:
		return X
	default:
		return Empty
	}
}
