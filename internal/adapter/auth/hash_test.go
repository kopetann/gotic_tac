package auth_test

import (
	"testing"

	"github.com/kopetann/gotic_tac/internal/adapter/auth"
	"github.com/kopetann/gotic_tac/internal/usecase"
)

var _ usecase.Hasher = auth.HashProvider{}

func TestHashProvider(t *testing.T) {
	hasher := auth.HashProvider{}

	hash, err := hasher.Hash("password123")
	if err != nil {
		t.Fatalf("Failed to hash a password: %s", err.Error())
	}

	if hash == "password123" {
		t.Fatal("Hash returned the password unchanged")
	}

	if err := hasher.Compare(hash, "password123"); err != nil {
		t.Errorf("Compare rejected the correct password: %s", err.Error())
	}

	if err := hasher.Compare(hash, "password124"); err == nil {
		t.Error("Compare accepted a wrong password")
	}
}

// bcrypt salts every call, so the same password never produces the same hash
// and Compare is the only way to check one.
func TestHashProviderSaltsEachCall(t *testing.T) {
	hasher := auth.HashProvider{}

	first, err := hasher.Hash("password123")
	if err != nil {
		t.Fatalf("Failed to hash a password: %s", err.Error())
	}

	second, err := hasher.Hash("password123")
	if err != nil {
		t.Fatalf("Failed to hash a password: %s", err.Error())
	}

	if first == second {
		t.Error("Two hashes of the same password are identical, want a fresh salt each time")
	}

	if err := hasher.Compare(second, "password123"); err != nil {
		t.Errorf("Compare rejected the correct password: %s", err.Error())
	}
}
