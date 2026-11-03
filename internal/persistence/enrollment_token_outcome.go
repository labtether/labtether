package persistence

import (
	"fmt"
	"time"

	"github.com/labtether/labtether/internal/enrollment"
)

// Reasons an enrollment token cannot be used. Each wraps
// ErrEnrollmentTokenInvalid, so callers that only need "not usable" keep
// working, while the Hub can tell the person running the installer which one
// it is. A token LabTether does not know stays the plain
// ErrEnrollmentTokenInvalid.
var (
	ErrEnrollmentTokenUsed    = fmt.Errorf("%w: it was already used", ErrEnrollmentTokenInvalid)
	ErrEnrollmentTokenExpired = fmt.Errorf("%w: it expired", ErrEnrollmentTokenInvalid)
	ErrEnrollmentTokenRevoked = fmt.Errorf("%w: it was revoked", ErrEnrollmentTokenInvalid)
)

// classifyUnusableEnrollmentToken names why a stored token failed validation.
func classifyUnusableEnrollmentToken(token enrollment.EnrollmentToken, found bool, now time.Time) error {
	switch {
	case !found:
		return ErrEnrollmentTokenInvalid
	case token.RevokedAt != nil:
		return ErrEnrollmentTokenRevoked
	case !now.Before(token.ExpiresAt):
		return ErrEnrollmentTokenExpired
	case token.MaxUses >= 1 && token.UseCount >= token.MaxUses:
		return ErrEnrollmentTokenUsed
	}
	return ErrEnrollmentTokenInvalid
}

// readmitsRetiredIdentity reports whether an enrollment token may bring back
// the name of a device that was removed (owner decision, 2026-11-03): only a
// single-use token made after the removal. Only admins can make tokens. The
// removed device's agent tokens stay revoked, so its old credential never
// works again; the device gets a new one.
func readmitsRetiredIdentity(token enrollment.EnrollmentToken, retiredAt time.Time) bool {
	return token.MaxUses == 1 && token.CreatedAt.After(retiredAt)
}
