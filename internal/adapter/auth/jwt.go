package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kopetann/gotic_tac/internal/domain"
)

type AuthProvider struct {
	secret 	   []byte
	encAlg     jwt.SigningMethod
	ttl 	   time.Duration
}

func NewAuthProvider(privateKey string, ttl time.Duration) (*AuthProvider, error) {
	if privateKey == "" {
		return nil, fmt.Errorf("Private key could not be empty")
	}

	return &AuthProvider{
		secret: []byte(privateKey),
		encAlg:     jwt.SigningMethodHS256,
		ttl: ttl,
	}, nil
}

func (a AuthProvider) Issue(id domain.PlayerID) (string, error) {
	now := time.Now()

	tok := jwt.NewWithClaims(a.encAlg, jwt.RegisteredClaims{
		Subject: string(id),
		IssuedAt: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(a.ttl)),
	})

	return tok.SignedString(a.secret)
}

func (a AuthProvider) Verify(token string) (domain.PlayerID, error) {
	tok, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Incorrect signing alg: %s", t.Method)
		}

		sub, err := t.Claims.GetSubject()
		if err != nil {
			return nil, err
		}

		if sub == "" {
			return nil, fmt.Errorf("Subject could not be empty")

		}

		return a.secret, nil
	},
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithValidMethods([]string{"HS256"}),
	)

	if err != nil {
		return "", err
	}

	userId, err := tok.Claims.GetSubject()
	if err != nil {
		return "", err
	}

	if userId == "" {
		return "", fmt.Errorf("Incorrect user_id")
	}

	return domain.PlayerID(userId), nil
}
