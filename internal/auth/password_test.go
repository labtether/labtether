package auth

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestDummyPasswordHashUsesProductionCost(t *testing.T) {
	cost, err := bcrypt.Cost([]byte(dummyPasswordHash))
	if err != nil {
		t.Fatalf("dummy password hash is invalid: %v", err)
	}
	if cost != bcryptCost {
		t.Fatalf("dummy password cost = %d, want %d", cost, bcryptCost)
	}
	if CheckDummyPassword("any-password") {
		t.Fatal("dummy password comparison must never authenticate")
	}
}
