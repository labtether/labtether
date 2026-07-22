package auth

import "golang.org/x/crypto/bcrypt"

const bcryptCost = 12

// dummyPasswordHash is a valid cost-12 bcrypt hash used to equalize the work
// performed for unknown and non-local usernames. Its plaintext is irrelevant;
// it is never accepted as an account credential.
const dummyPasswordHash = "$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6Ttx3n1A36mPPwMcF5g5KkMkTJFGu" // #nosec G101 -- Deliberately public dummy hash; never accepted as an account credential.

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
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
	_ = bcrypt.CompareHashAndPassword([]byte(dummyPasswordHash), []byte(password))
	return false
}
