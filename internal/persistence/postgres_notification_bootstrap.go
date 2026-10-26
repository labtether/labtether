package persistence

import (
	"context"
	"strings"
)

// ClaimNotificationBootstrap records a one-time automatic notification setup
// step. Exactly one caller, across Hub processes, gets true for a name.
func (s *PostgresStore) ClaimNotificationBootstrap(ctx context.Context, name string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO notification_bootstrap_markers (name) VALUES ($1)
		ON CONFLICT (name) DO NOTHING
	`, strings.TrimSpace(name))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// ReleaseNotificationBootstrap undoes a claim whose setup step failed, so the
// next trigger can try again.
func (s *PostgresStore) ReleaseNotificationBootstrap(ctx context.Context, name string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM notification_bootstrap_markers WHERE name = $1`, strings.TrimSpace(name))
	return err
}
