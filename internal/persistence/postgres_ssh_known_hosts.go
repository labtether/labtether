package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/terminal"
)

func (s *PostgresStore) GetSSHKnownHost(host string, port int) (terminal.SSHKnownHost, bool, error) {
	host, port = terminal.SSHPinHost(host, port)
	var known terminal.SSHKnownHost
	err := s.pool.QueryRow(context.Background(),
		`SELECT host, port, host_key, fingerprint, pinned_by, pinned_at
		 FROM ssh_known_hosts WHERE host = $1 AND port = $2`,
		host, port,
	).Scan(&known.Host, &known.Port, &known.HostKey, &known.Fingerprint, &known.PinnedBy, &known.PinnedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return terminal.SSHKnownHost{}, false, nil
	}
	if err != nil {
		return terminal.SSHKnownHost{}, false, err
	}
	return known, true, nil
}

// AddSSHKnownHost never replaces a stored key: a key already stored for the
// host and port (also one written at the same moment by someone else) is
// returned instead, so the caller can see that it differs.
func (s *PostgresStore) AddSSHKnownHost(known terminal.SSHKnownHost) (terminal.SSHKnownHost, error) {
	known.Host, known.Port = terminal.SSHPinHost(known.Host, known.Port)
	if known.PinnedAt.IsZero() {
		known.PinnedAt = time.Now().UTC()
	}
	if _, err := s.pool.Exec(context.Background(),
		`INSERT INTO ssh_known_hosts (host, port, host_key, fingerprint, pinned_by, pinned_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (host, port) DO NOTHING`,
		known.Host, known.Port, known.HostKey, known.Fingerprint, known.PinnedBy, known.PinnedAt,
	); err != nil {
		return terminal.SSHKnownHost{}, err
	}
	stored, ok, err := s.GetSSHKnownHost(known.Host, known.Port)
	if err != nil {
		return terminal.SSHKnownHost{}, err
	}
	if !ok {
		return terminal.SSHKnownHost{}, ErrNotFound
	}
	return stored, nil
}

func (s *PostgresStore) DeleteSSHKnownHost(host string, port int) (bool, error) {
	host, port = terminal.SSHPinHost(host, port)
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM ssh_known_hosts WHERE host = $1 AND port = $2`, host, port)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
