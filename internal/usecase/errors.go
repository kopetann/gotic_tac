package usecase

import (
	"fmt"

	"github.com/kopetann/gotic_tac/internal/domain"
)

var (
	ErrPlayerNotFound     = &domain.Error{Code: "PLAYER_NOT_FOUND", Message: "player not found"}
	ErrNameTaken          = &domain.Error{Code: "NAME_TAKEN", Message: "that name is already registered"}
	ErrInvalidCredentials = &domain.Error{Code: "INVALID_CREDENTIALS", Message: "invalid name or password"}
	ErrPasswordTooShort   = &domain.Error{
		Code:    "PASSWORD_TOO_SHORT",
		Message: fmt.Sprintf("password must be at least %d characters", MinPasswordLength),
	}
	ErrPasswordTooLong = &domain.Error{
		Code:    "PASSWORD_TOO_LONG",
		Message: fmt.Sprintf("password must have a length mo more than %d characters long", MaxPasswordLength),
	}
	ErrInvalidToken   = &domain.Error{Code: "INVALID_TOKEN", Message: "invalid or expired token"}
	ErrAlreadyQueued  = &domain.Error{Code: "ALREADY_QUEUED", Message: "you are already waiting for an opponent"}
	ErrAlreadyInMatch = &domain.Error{Code: "ALREADY_IN_MATCH", Message: "you are already playing a match"}
	ErrNoActiveMatch  = &domain.Error{Code: "NO_ACTIVE_MATCH", Message: "you are not in a match"}
)
