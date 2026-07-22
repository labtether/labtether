package auth

import (
	"net/http"

	coreauth "github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/persistence"
)

// revokeOtherUserSessions revokes every session for userID except the exact
// cookie session authenticating the request. The returned boolean reports
// whether that current session was found and retained. Package-level callers
// without a cookie revoke the full set instead of guessing which session is
// safe to preserve.
func revokeOtherUserSessions(store persistence.AuthStore, userID string, r *http.Request) (bool, error) {
	if store == nil {
		return false, nil
	}
	token := coreauth.ExtractSessionToken(r)
	if token == "" {
		return false, store.DeleteSessionsByUserID(userID)
	}
	currentHash := coreauth.HashToken(token)
	sessions, err := store.ListSessionsByUserID(userID)
	if err != nil {
		return false, err
	}
	retainedCurrent := false
	for _, session := range sessions {
		if session.TokenHash == currentHash {
			retainedCurrent = true
			continue
		}
		if err := store.DeleteSession(session.ID); err != nil {
			return false, err
		}
	}
	return retainedCurrent, nil
}
