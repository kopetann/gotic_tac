package transport

import (
	"github.com/kopetann/gotic_tac/internal/domain"
	"github.com/kopetann/gotic_tac/internal/usecase"
)

// Commands a client may send over the socket.
const (
	cmdJoin   = "join"
	cmdLeave  = "leave"
	cmdMove   = "move"
	cmdState  = "state"
	cmdResign = "resign"
)

// Frames the server sends that are not usecase events.
const (
	frameAck   = "ack"
	frameError = "error"
)

type command struct {
	Type string `json:"type"`
	Cell int    `json:"cell"`
}

type frame struct {
	Type    string      `json:"type"`
	Of      string      `json:"of,omitempty"`
	MatchID string      `json:"match_id,omitempty"`
	State   *matchState `json:"state,omitempty"`
	Code    string      `json:"code,omitempty"`
	Message string      `json:"message,omitempty"`
}

type matchState struct {
	MatchID  string                   `json:"match_id"`
	Cells    [domain.BoardSize]string `json:"cells"`
	YourMark string                   `json:"your_mark"`
	Turn     string                   `json:"turn"`
	Status   string                   `json:"status"`
	Outcome  string                   `json:"outcome"`
	Opponent string                   `json:"opponent"`
	Finished bool                     `json:"finished"`
}

// HTTP bodies.
type (
	credentials struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}

	tokenResponse struct {
		Token string `json:"token"`
	}

	moveRequest struct {
		Cell int `json:"cell"`
	}

	standing struct {
		Rank   int    `json:"rank"`
		Name   string `json:"name"`
		Wins   int    `json:"wins"`
		Losses int    `json:"losses"`
		Draws  int    `json:"draws"`
	}

	leaderboardResponse struct {
		Standings []standing `json:"standings"`
	}

	queueResponse struct {
		Waiting int `json:"waiting"`
	}
)

// markString is not domain.Mark.String: that returns " " for Empty, which is
// right for a printed grid and wrong for JSON.
func markString(m domain.Mark) string {
	switch m {
	case domain.X:
		return "X"
	case domain.O:
		return "O"
	default:
		return ""
	}
}

func outcomeString(o domain.Outcome) string {
	switch o {
	case domain.OutcomeWin:
		return "win"
	case domain.OutcomeLoss:
		return "loss"
	case domain.OutcomeDraw:
		return "draw"
	default:
		return "undecided"
	}
}

func cells(b domain.Board) [domain.BoardSize]string {
	var out [domain.BoardSize]string

	for i := range domain.BoardSize {
		out[i] = markString(b.Cell(i))
	}

	return out
}

func newMatchState(s usecase.MatchState) *matchState {
	return &matchState{
		MatchID:  string(s.MatchID),
		Cells:    cells(s.Cells),
		YourMark: markString(s.YourMark),
		Turn:     markString(s.Turn),
		Status:   s.Status.String(),
		Outcome:  outcomeString(s.Outcome),
		Opponent: s.Opponent,
		Finished: s.Status.IsFinished(),
	}
}

func eventFrame(ev usecase.Event) frame {
	return frame{
		Type:    string(ev.Type),
		MatchID: string(ev.MatchID),
		State:   newMatchState(ev.State),
	}
}
