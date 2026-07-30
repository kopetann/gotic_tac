package usecase

import "github.com/kopetann/gotic_tac/internal/domain"

type EventType string

const (
	EventMatchStarted EventType = "match_started"
	EventStateChanged EventType = "state_changed"
	EventMatchEnded   EventType = "match_ended"
	EventOpponentLeft EventType = "opponent_left"
)

// MatchState is what one player is allowed to know: their own mark and their
// own outcome, never the opponent's identity beyond a display name.
type MatchState struct {
	MatchID  domain.MatchID
	Cells    domain.Board
	YourMark domain.Mark
	Turn     domain.Mark
	Status   domain.Status
	Outcome  domain.Outcome
	Opponent string
}

type Event struct {
	Type    EventType
	MatchID domain.MatchID
	State   MatchState
}
