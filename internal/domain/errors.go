package domain

type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

var (
	ErrCellOutOfBounds = &Error{Code: "CELL_OUT_OF_BOUNDS", Message: "cell must be between 0 and 8"}
	ErrCellOccupied    = &Error{Code: "CELL_OCCUPIED", Message: "cell is already taken"}
	ErrNotYourTurn     = &Error{Code: "NOT_YOUR_TURN", Message: "it is not your turn"}
	ErrMatchFinished   = &Error{Code: "MATCH_FINISHED", Message: "the match is already over"}
	ErrNotAParticipant = &Error{Code: "NOT_A_PARTICIPANT", Message: "you are not a player in this match"}
	ErrEmptyName       = &Error{Code: "EMPTY_NAME", Message: "player name must not be empty"}
	ErrNameTooLong     = &Error{Code: "NAME_TOO_LONG", Message: "player name must be at most 32 characters"}
	ErrSamePlayer      = &Error{Code: "SAME_PLAYER", Message: "a player cannot be matched against themselves"}
)
