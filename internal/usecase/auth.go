package usecase

import (
	"context"
	"errors"

	"github.com/kopetann/gotic_tac/internal/domain"
)

// MinPasswordLength lives here rather than in the domain because the domain
// never sees a plaintext password, only the hash of one.
const MinPasswordLength = 8

type Auth struct {
	players PlayerRegistry
	hasher  Hasher
	tokens  TokenIssuer
	ids     IDGenerator
}

func NewAuth(players PlayerRegistry, hasher Hasher, tokens TokenIssuer, ids IDGenerator) *Auth {
	return &Auth{players: players, hasher: hasher, tokens: tokens, ids: ids}
}

func (a *Auth) Register(ctx context.Context, name, password string) (string, error) {
	name, err := domain.NormalizeName(name)
	if err != nil {
		return "", err
	}

	if len(password) < MinPasswordLength {
		return "", ErrPasswordTooShort
	}

	// Three outcomes, not two: found means taken, not-found means free, and any
	// other error is a storage failure that must not be read as "name is free".
	switch _, err := a.players.ByName(ctx, name); {
	case err == nil:
		return "", ErrNameTaken
	case !errors.Is(err, ErrPlayerNotFound):
		return "", err
	}

	hash, err := a.hasher.Hash(password)
	if err != nil {
		return "", err
	}

	p, err := domain.NewPlayer(domain.PlayerID(a.ids.NewID()), name, hash)
	if err != nil {
		return "", err
	}

	if err := a.players.Create(ctx, p); err != nil {
		return "", err
	}

	return a.tokens.Issue(p.ID)
}

func (a *Auth) Login(ctx context.Context, name, password string) (string, error) {
	name, err := domain.NormalizeName(name)
	if err != nil {
		return "", ErrInvalidCredentials
	}

	// An unknown name and a wrong password return the same error, so login
	// cannot be used to discover who has an account.
	p, err := a.players.ByName(ctx, name)
	if err != nil {
		if errors.Is(err, ErrPlayerNotFound) {
			return "", ErrInvalidCredentials
		}

		return "", err
	}

	if err := a.hasher.Compare(p.PasswordHash, password); err != nil {
		return "", ErrInvalidCredentials
	}

	return a.tokens.Issue(p.ID)
}

// Authenticate collapses every verification failure into one error: telling a
// client whether a token was expired or forged helps nobody but an attacker.
func (a *Auth) Authenticate(token string) (domain.PlayerID, error) {
	id, err := a.tokens.Verify(token)
	if err != nil {
		return "", ErrInvalidToken
	}

	return id, nil
}
