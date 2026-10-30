package persistence

import (
	"context"
	"fmt"
)

// EncryptedSecretsState describes the stored values that depend on the Hub's
// encryption key. Startup reads it to refuse to start with a new key while
// such values exist, because a new key cannot read them.
type EncryptedSecretsState struct {
	// Credentials is true when a saved credential (including the Hub's own SSH
	// identity) or a webhook signing secret is stored.
	Credentials bool
	// TOTP is true when a user has a 2FA enrolment.
	TOTP bool
}

// LoadEncryptedSecretsState reads EncryptedSecretsState.
func (s *PostgresStore) LoadEncryptedSecretsState(ctx context.Context) (EncryptedSecretsState, error) {
	var state EncryptedSecretsState
	if err := s.pool.QueryRow(ctx, `SELECT
			EXISTS (SELECT 1 FROM credential_profiles WHERE secret_ciphertext <> '')
				OR EXISTS (SELECT 1 FROM webhooks WHERE secret_ciphertext <> ''),
			EXISTS (SELECT 1 FROM users WHERE COALESCE(totp_secret, '') <> '')`,
	).Scan(&state.Credentials, &state.TOTP); err != nil {
		return EncryptedSecretsState{}, fmt.Errorf("check for encrypted secrets: %w", err)
	}
	return state, nil
}
