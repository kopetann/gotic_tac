package auth_test

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kopetann/gotic_tac/internal/adapter/auth"
	"github.com/kopetann/gotic_tac/internal/domain"
	"github.com/kopetann/gotic_tac/internal/usecase"
)

var _ usecase.TokenIssuer = (*auth.AuthProvider)(nil)

const (
	testSecret  = "test-secret-that-is-32-bytes-long"
	otherSecret = "different-secret-32-bytes-long!!!"
	testPlayer  = domain.PlayerID("player-1")
)

func setupAuthProvider(t *testing.T, ttl time.Duration) *auth.AuthProvider {
	t.Helper()

	authProvider, err := auth.NewAuthProvider(testSecret, ttl)
	if err != nil {
		t.Fatalf("Failed to build AuthProvider: %s", err.Error())
	}

	return authProvider
}

func issue(t *testing.T, a *auth.AuthProvider, id domain.PlayerID) string {
	t.Helper()

	tok, err := a.Issue(id)
	if err != nil {
		t.Fatalf("Failed to issue token: %s", err.Error())
	}

	return tok
}

// signWith mints a token outside AuthProvider so the algorithm and key can differ.
func signWith(t *testing.T, method jwt.SigningMethod, key any) string {
	t.Helper()

	now := time.Now()
	tok, err := jwt.NewWithClaims(method, jwt.RegisteredClaims{
		Subject:   string(testPlayer),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
	}).SignedString(key)

	if err != nil {
		t.Fatalf("Failed to sign token: %s", err.Error())
	}

	return tok
}

// tamper rewrites the subject in the payload and leaves the signature alone.
func tamper(t *testing.T, token string) string {
	t.Helper()

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("Token has %d segments, want 3", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("Failed to decode payload: %s", err.Error())
	}

	parts[1] = base64.RawURLEncoding.EncodeToString(
		bytes.Replace(payload, []byte("player-1"), []byte("player-2"), 1),
	)

	return strings.Join(parts, ".")
}

func TestAuthProvider(t *testing.T) {
	authProvider := setupAuthProvider(t, time.Minute*5)

	userId, err := authProvider.Verify(issue(t, authProvider, testPlayer))
	if err != nil {
		t.Fatalf("Failed to verify token: %s", err.Error())
	}

	if userId != testPlayer {
		t.Errorf("Verify() = %q, want %q", userId, testPlayer)
	}
}

func TestAuthProviderRejects(t *testing.T) {
	authProvider := setupAuthProvider(t, time.Minute*5)

	tests := []struct {
		name  string
		token func(t *testing.T) string
	}{
		{"malformed", func(*testing.T) string {
			return "not.a.token"
		}},

		{"expired", func(t *testing.T) string {
			return issue(t, setupAuthProvider(t, -time.Minute), testPlayer)
		}},

		{"another secret", func(t *testing.T) string {
			return signWith(t, jwt.SigningMethodHS256, []byte(otherSecret))
		}},

		{"another algorithm", func(t *testing.T) string {
			return signWith(t, jwt.SigningMethodHS512, []byte(testSecret))
		}},

		{"no signature", func(t *testing.T) string {
			return signWith(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType)
		}},

		{"tampered subject", func(t *testing.T) string {
			return tamper(t, issue(t, authProvider, testPlayer))
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userId, err := authProvider.Verify(tt.token(t))
			if err == nil {
				t.Fatalf("Verify() error = nil, want error")
			}

			if userId != "" {
				t.Errorf("Verify() = %q, want empty on failure", userId)
			}
		})
	}
}

func TestNewAuthProviderRejectsEmptySecret(t *testing.T) {
	if _, err := auth.NewAuthProvider("", time.Minute); err == nil {
		t.Error("NewAuthProvider() error = nil, want error")
	}
}
