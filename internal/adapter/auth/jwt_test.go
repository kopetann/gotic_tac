package auth_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kopetann/gotic_tac/internal/adapter/auth"
	"github.com/kopetann/gotic_tac/internal/domain"
)

func TestAuthProvider(t *testing.T) {
	authProvider := setupAuthProvider(t)

	randId := uuid.New().String()

	tok, err := authProvider.Issue(domain.PlayerID(randId))
	if err != nil {
		t.Fatalf("Failed to Issue token at AuthService: %s", err.Error())
	}

	userId, err := authProvider.Verify(tok)
	if err != nil {
		t.Fatalf("Failed to verify token at AuthService: %s", err.Error())
	}

	if userId != domain.PlayerID(randId) {
		t.Fatalf("Validation returned wrong or empty userId")
	}
}

func setupAuthProvider(t *testing.T) auth.AuthProvider {
	t.Helper()

	ttl := time.Minute * 5
	secret := "test_secret"

	authProvider, err := auth.NewAuthProvider(secret, ttl)
	if err != nil {
		t.Fatalf("Failed to test authService: %s", err.Error())
	}

	return *authProvider
}
