package domain

import "strings"

const MaxNameLength = 32

// Stats holds all the player info for the wins, losses and draws
type Stats struct {
	Wins   int
	Losses int
	Draws  int
}

func (s Stats) Played() int { return s.Wins + s.Losses + s.Draws }

type Player struct {
	ID           PlayerID
	Name         string
	PasswordHash string
	Stats        Stats
}

func NormalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrEmptyName
	}
	if len(name) > MaxNameLength {
		return "", ErrNameTooLong
	}
	return name, nil
}

func NewPlayer(id PlayerID, name, passwordHash string) (*Player, error) {
	name, err := NormalizeName(name)
	if err != nil {
		return nil, err
	}

	return &Player{
		ID:           id,
		Name:         name,
		PasswordHash: passwordHash,
	}, nil
}

func (p *Player) Record(o Outcome) {
	switch o {
	case OutcomeWin:
		p.Stats.Wins++

	case OutcomeLoss:
		p.Stats.Losses++

	case OutcomeDraw:
		p.Stats.Draws++
	}
}
