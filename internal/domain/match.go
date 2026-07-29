package domain

type (
	MatchID  string
	PlayerID string
)

type Status uint8

const (
	StatusInProgress Status = iota
	StatusXWon
	StatusOWon
	StatusDraw
)

func (s Status) IsFinished() bool {
	return s != StatusInProgress
}

func (s Status) String() string {
	switch s {
	case StatusXWon:
		return "x_won"
	case StatusOWon:
		return "o_won"
	case StatusDraw:
		return "draw"
	default:
		return "in_progress"
	}
}

type Outcome uint8

const (
	OutcomeUndecided Outcome = iota
	OutcomeWin
	OutcomeLoss
	OutcomeDraw
)

type Move struct {
	Mark Mark
	Cell int
}

// Match is the aggregate root: it owns the board and is the only thing allowed
// to change it. Every rule in the assessment brief — alternating turns, no
// occupied cells, no out-of-bounds, no moves after the game ends, correct
// win/draw detection — is enforced here and nowhere else.
//
// Deliberately not safe for concurrent use. Serialising access to a match is a
// different responsibility, handled one layer out by the session that owns it.
// Mixing a mutex in here would give the entity a second reason to change.
type Match struct {
	id      MatchID
	players map[Mark]PlayerID
	board   Board
	turn    Mark
	status  Status
	moves   []Move
}

// NewMatch starts a match between two distinct players. X always moves first.
func NewMatch(id MatchID, x, o PlayerID) (*Match, error) {
	if x == o {
		return nil, ErrSamePlayer
	}
	
	return &Match{
		id:      id,
		players: map[Mark]PlayerID{X: x, O: o},
		turn:    X,
		status:  StatusInProgress,
	}, nil
}

// RestoreMatch rebuilds a match from its persisted move list.
//
// It replays the moves through ApplyMove rather than assigning the board
// directly, so a match loaded from the database is validated by exactly the
// same rules as one played live. Corrupt history fails loudly instead of
// producing an impossible position.
func RestoreMatch(id MatchID, x, o PlayerID, moves []Move) (*Match, error) {
	m, err := NewMatch(id, x, o)
	if err != nil {
		return nil, err
	}

	for _, mv := range moves {
		if err := m.ApplyMove(m.players[mv.Mark], mv.Cell); err != nil {
			return nil, err
		}
	}

	return m, nil
}

// ApplyMove places the calling player's mark on cell, advancing the turn or
// finishing the match. The checks are ordered from most general to most
// specific so the caller always gets the most informative error.
func (m *Match) ApplyMove(p PlayerID, cell int) error {
	if m.status.IsFinished() {
		return ErrMatchFinished
	}

	mark, ok := m.MarkOf(p)
	if !ok {
		return ErrNotAParticipant
	}

	if mark != m.turn {
		return ErrNotYourTurn
	}

	if err := m.board.Set(cell, mark); err != nil {
		return err
	}

	m.moves = append(m.moves, Move{Mark: mark, Cell: cell})
	m.settle(mark)
	return nil
}

// settle records the result if the move just played ended the match, and
// otherwise passes the turn.
func (m *Match) settle(justPlayed Mark) {
	winner := m.board.IsWinningCombination()
	switch {
	case winner == X:
		m.status = StatusXWon

	case winner == O:
		m.status = StatusOWon

	case m.board.IsFull():
		m.status = StatusDraw

	default:
		m.turn = justPlayed.Opponent()
	}

}

// MarkOf reports which side p plays, and whether p is in this match at all.
func (m *Match) MarkOf(p PlayerID) (Mark, bool) {
	for mark, id := range m.players {
		if id == p {
			return mark, true
		}
	}

	return Empty, false
}

// PlayerOf reports who plays the given mark.
func (m *Match) PlayerOf(mark Mark) PlayerID { return m.players[mark] }

// Opponent returns the other participant's ID.
func (m *Match) Opponent(p PlayerID) PlayerID {
	mark, ok := m.MarkOf(p)
	if !ok {
		return ""
	}

	return m.players[mark.Opponent()]
}

// OutcomeFor expresses the result from p's point of view, which is what the
// leaderboard and the end-of-game message both need.
func (m *Match) OutcomeFor(p PlayerID) Outcome {
	if !m.status.IsFinished() {
		return OutcomeUndecided
	}

	if m.status == StatusDraw {
		return OutcomeDraw
	}

	mark, ok := m.MarkOf(p)
	if !ok {
		return OutcomeUndecided
	}

	if (m.status == StatusXWon && mark == X) || (m.status == StatusOWon && mark == O) {
		return OutcomeWin
	}

	return OutcomeLoss
}

// Winner returns the winning player and true, or "" and false for an unfinished match or a draw.
func (m *Match) Winner() (PlayerID, bool) {
	switch m.status {
	case StatusXWon:
		return m.players[X], true

	case StatusOWon:
		return m.players[O], true

	default:
		return "", false
	}
}

func (m *Match) ID() MatchID {
	return m.id
}

func (m *Match) Board() Board {
	return m.board
}

func (m *Match) Turn() Mark {
	return m.turn
}

func (m *Match) Status() Status {
	return m.status
}

// Moves returns a defensive copy so callers cannot rewrite history.
func (m *Match) Moves() []Move {
	out := make([]Move, len(m.moves))
	copy(out, m.moves)

	return out
}
