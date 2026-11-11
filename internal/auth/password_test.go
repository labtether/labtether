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

// Guards the stored-credential promise: outside a test binary that opted in to
// fast hashing, passwords and recovery codes are hashed at the production cost.
func TestHashesUseProductionCostByDefault(t *testing.T) {
	passwordHash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	recoveryHash, err := HashRecoveryCode("abcd1234-ef567890")
	if err != nil {
		t.Fatalf("hash recovery code: %v", err)
	}
	for name, tc := range map[string]struct {
		hash string
		want int
	}{
		"password":      {passwordHash, bcryptCost},
		"recovery code": {recoveryHash, recoveryCodeBcryptCost},
	} {
		cost, err := bcrypt.Cost([]byte(tc.hash))
		if err != nil {
			t.Fatalf("%s hash is invalid: %v", name, err)
		}
		if cost != tc.want {
			t.Fatalf("%s cost = %d, want %d", name, cost, tc.want)
		}
	}
}
