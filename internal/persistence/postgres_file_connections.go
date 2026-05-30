package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

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
	now := time.Now().UTC()
	fc.UpdatedAt = now

	configPayload, err := marshalAnyMap(fc.ExtraConfig)
	if err != nil {
		return err
	}

	tag, err := s.pool.Exec(ctx,
		`UPDATE file_connections
			 SET actor_id = $2, name = $3, protocol = $4, host = $5, port = $6,
			     initial_path = $7, credential_id = $8, extra_config = $9::jsonb, updated_at = $10
			 WHERE id = $1`,
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
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
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
