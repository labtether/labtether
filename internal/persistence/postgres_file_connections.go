package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/idgen"
)

// --- scan helpers ---

type fileConnectionScanner interface {
	Scan(dest ...any) error
}

func scanFileConnection(row fileConnectionScanner) (FileConnection, error) {
	fc := FileConnection{}
	var extraConfig []byte
	if err := row.Scan(
		&fc.ID,
		&fc.ActorID,
		&fc.Name,
		&fc.Protocol,
		&fc.Host,
		&fc.Port,
		&fc.InitialPath,
		&fc.CredentialID,
		&extraConfig,
		&fc.CreatedAt,
		&fc.UpdatedAt,
	); err != nil {
		return FileConnection{}, err
	}
	fc.ExtraConfig = unmarshalAnyMap(extraConfig)
	fc.CreatedAt = fc.CreatedAt.UTC()
	fc.UpdatedAt = fc.UpdatedAt.UTC()
	return fc, nil
}

// --- columns ---

const fileConnectionColumns = `id, actor_id, name, protocol, host, port, initial_path, credential_id, extra_config, created_at, updated_at`

// --- store methods ---

func (s *PostgresStore) ListFileConnections(ctx context.Context) ([]FileConnection, error) {
	rows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM file_connections ORDER BY updated_at DESC`, fileConnectionColumns),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]FileConnection, 0, 16)
	for rows.Next() {
		fc, scanErr := scanFileConnection(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, fc)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetFileConnection(ctx context.Context, id string) (*FileConnection, error) {
	fc, err := scanFileConnection(s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM file_connections WHERE id = $1`, fileConnectionColumns),
		strings.TrimSpace(id),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &fc, nil
}

func (s *PostgresStore) CreateFileConnection(ctx context.Context, fc *FileConnection) error {
	now := time.Now().UTC()
	fc.ID = idgen.New("fconn")
	fc.CreatedAt = now
	fc.UpdatedAt = now

	if strings.TrimSpace(fc.InitialPath) == "" {
		fc.InitialPath = "/"
	}

	configPayload, err := marshalAnyMap(fc.ExtraConfig)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO file_connections (id, actor_id, name, protocol, host, port, initial_path, credential_id, extra_config, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11)`,
		fc.ID,
		strings.TrimSpace(fc.ActorID),
		strings.TrimSpace(fc.Name),
		strings.TrimSpace(fc.Protocol),
		strings.TrimSpace(fc.Host),
		fc.Port,
		fc.InitialPath,
		fc.CredentialID,
		configPayload,
		fc.CreatedAt,
		fc.UpdatedAt,
	)
	return err
}

func (s *PostgresStore) UpdateFileConnection(ctx context.Context, fc *FileConnection) error {
	return updateFileConnection(ctx, s.pool, fc)
}

type fileConnectionQueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func updateFileConnection(ctx context.Context, q fileConnectionQueryRower, fc *FileConnection) error {
	expectedUpdatedAt := fc.UpdatedAt
	now := time.Now().UTC()
	fc.UpdatedAt = now

	configPayload, err := marshalAnyMap(fc.ExtraConfig)
	if err != nil {
		return err
	}

	var storedConfig []byte
	err = q.QueryRow(ctx,
		`UPDATE file_connections
			 SET actor_id = $2, name = $3, protocol = $4, host = $5, port = $6,
			     initial_path = $7, credential_id = $8,
			     extra_config = CASE
			         WHEN $4 = 'sftp'
			          AND NOT ($9::jsonb ? 'host_key')
			          AND extra_config ? 'host_key'
			         THEN $9::jsonb || jsonb_build_object('host_key', extra_config -> 'host_key')
			         ELSE $9::jsonb
			     END,
			     updated_at = $10
			 WHERE id = $1 AND updated_at = $11
			 RETURNING extra_config`,
		strings.TrimSpace(fc.ID),
		strings.TrimSpace(fc.ActorID),
		strings.TrimSpace(fc.Name),
		strings.TrimSpace(fc.Protocol),
		strings.TrimSpace(fc.Host),
		fc.Port,
		fc.InitialPath,
		fc.CredentialID,
		configPayload,
		fc.UpdatedAt,
		expectedUpdatedAt,
	).Scan(&storedConfig)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			var exists bool
			if existsErr := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_connections WHERE id = $1)`, strings.TrimSpace(fc.ID)).Scan(&exists); existsErr != nil {
				return existsErr
			}
			if !exists {
				return ErrNotFound
			}
			return ErrFileConnectionChanged
		}
		return err
	}
	fc.ExtraConfig = unmarshalAnyMap(storedConfig)
	return nil
}

// UpdateFileConnectionWithCredential keeps file connection metadata and its
// encrypted credential profile in one transaction, including conflict paths.
func (s *PostgresStore) UpdateFileConnectionWithCredential(
	ctx context.Context,
	fc *FileConnection,
	profile credentials.Profile,
	createProfile bool,
) (credentials.Profile, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return credentials.Profile{}, err
	}
	defer tx.Rollback(ctx)

	var savedProfile credentials.Profile
	if createProfile {
		savedProfile, err = createCredentialProfile(ctx, tx, profile)
	} else {
		savedProfile, err = updateCredentialProfile(ctx, tx, profile)
	}
	if err != nil {
		return credentials.Profile{}, err
	}
	if err := updateFileConnection(ctx, tx, fc); err != nil {
		return credentials.Profile{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return credentials.Profile{}, err
	}
	return savedProfile, nil
}

// PinSFTPHostKey atomically records the first host key seen for a saved SFTP
// connection. A concurrent different key, or an endpoint edit during the SSH
// handshake, is rejected without overwriting the trusted key.
func (s *PostgresStore) PinSFTPHostKey(ctx context.Context, connectionID, expectedHost string, expectedPort int, presentedKey string) error {
	connectionID = strings.TrimSpace(connectionID)
	expectedHost = strings.TrimSpace(expectedHost)
	presentedKey = strings.TrimSpace(presentedKey)
	if connectionID == "" || expectedHost == "" || expectedPort <= 0 || presentedKey == "" {
		return ErrFileConnectionChanged
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var (
		protocol     string
		storedHost   string
		storedPort   *int
		extraPayload []byte
	)
	if err := tx.QueryRow(ctx,
		`SELECT protocol, host, port, extra_config
		 FROM file_connections
		 WHERE id = $1
		 FOR UPDATE`,
		connectionID,
	).Scan(&protocol, &storedHost, &storedPort, &extraPayload); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	actualPort := 22
	if storedPort != nil && *storedPort != 0 {
		actualPort = *storedPort
	}
	if strings.TrimSpace(protocol) != "sftp" || strings.TrimSpace(storedHost) != expectedHost || actualPort != expectedPort {
		return ErrFileConnectionChanged
	}

	extraConfig := unmarshalAnyMap(extraPayload)
	if extraConfig == nil {
		extraConfig = map[string]any{}
	}
	if existing, exists := extraConfig["host_key"]; exists {
		existingKey, ok := existing.(string)
		if !ok || strings.TrimSpace(existingKey) == "" || strings.TrimSpace(existingKey) != presentedKey {
			return ErrSFTPHostKeyMismatch
		}
		return tx.Commit(ctx)
	}

	extraConfig["host_key"] = presentedKey
	updatedPayload, err := marshalAnyMap(extraConfig)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE file_connections
		 SET extra_config = $2::jsonb, updated_at = NOW()
		 WHERE id = $1`,
		connectionID,
		updatedPayload,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) DeleteFileConnection(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM file_connections WHERE id = $1`,
		strings.TrimSpace(id),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
