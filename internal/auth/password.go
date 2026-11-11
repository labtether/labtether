package auth

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

const (
	bcryptCost             = 12
	recoveryCodeBcryptCost = 10
)

// dummyPasswordHash is a valid cost-12 bcrypt hash used to equalize the work
// performed for unknown and non-local usernames. Its plaintext is irrelevant;
// it is never accepted as an account credential.
const dummyPasswordHash = "$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6Ttx3n1A36mPPwMcF5g5KkMkTJFGu" // #nosec G101 -- Deliberately public dummy hash; never accepted as an account credential.

// These hold the values the functions below use. They differ from the
// constants above only in a test binary that called
// UseFastPasswordHashingInTests.
var (
	passwordCost     = bcryptCost
	recoveryCodeCost = recoveryCodeBcryptCost
	dummyHash        = dummyPasswordHash
)

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), passwordCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func CheckPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// CheckDummyPassword performs one normal bcrypt comparison and always returns
// false. Callers use it on lookup/provider failures to avoid username timing
// disclosure.
func CheckDummyPassword(password string) bool {
	_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
	return false
}

// UseFastPasswordHashingInTests lowers every bcrypt work factor in this
// package to the minimum for the rest of the test binary. Under the race
// detector one cost-12 hash takes seconds, which made the login handler tests
// most of the Go test time in CI. Call it from a test package's init, before
// any test runs. It panics outside a test binary, so a built Hub always hashes
// at the costs above.
func UseFastPasswordHashingInTests() {
	if !testing.Testing() {
		panic("auth: password hashing cost can only be lowered in a test binary")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("dummy"), bcrypt.MinCost)
	if err != nil {
		panic(err)
	}
	passwordCost = bcrypt.MinCost
	recoveryCodeCost = bcrypt.MinCost
	dummyHash = string(hash)
}
