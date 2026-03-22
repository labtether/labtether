package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/idgen"
)

// --- scan helpers ---

type fileTransferScanner interface {
	Scan(dest ...any) error
}

func scanFileTransfer(row fileTransferScanner) (FileTransfer, error) {
	ft := FileTransfer{}
	if err := row.Scan(
		&ft.ID,
		&ft.SourceType,
		&ft.SourceID,
		&ft.SourcePath,
		&ft.DestType,
		&ft.DestID,
		&ft.DestPath,
		&ft.FileName,
		&ft.FileSize,
		&ft.BytesTransferred,
		&ft.Status,
		&ft.Error,
		&ft.StartedAt,
		&ft.CompletedAt,
	); err != nil {
		return FileTransfer{}, err
	}
	if ft.StartedAt != nil {
		utc := ft.StartedAt.UTC()
		ft.StartedAt = &utc
	}
	if ft.CompletedAt != nil {
		utc := ft.CompletedAt.UTC()
		ft.CompletedAt = &utc
	}
	return ft, nil
}

// --- columns ---

const fileTransferColumns = `id, source_type, source_id, source_path, dest_type, dest_id, dest_path, file_name, file_size, bytes_transferred, status, error, started_at, completed_at`

// --- store methods ---

func (s *PostgresStore) GetFileTransfer(ctx context.Context, id string) (*FileTransfer, error) {
	ft, err := scanFileTransfer(s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM file_transfers WHERE id = $1`, fileTransferColumns),
		strings.TrimSpace(id),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &ft, nil
}

func (s *PostgresStore) CreateFileTransfer(ctx context.Context, ft *FileTransfer) error {
	ft.ID = idgen.New("ftx")
	if ft.Status == "" {
		ft.Status = "pending"
	}

	_, err := s.pool.Exec(ctx,
		`INSERT INTO file_transfers (id, source_type, source_id, source_path, dest_type, dest_id, dest_path, file_name, file_size, bytes_transferred, status, error, started_at, completed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		ft.ID,
		strings.TrimSpace(ft.SourceType),
		strings.TrimSpace(ft.SourceID),
		ft.SourcePath,
		strings.TrimSpace(ft.DestType),
		strings.TrimSpace(ft.DestID),
		ft.DestPath,
		ft.FileName,
		ft.FileSize,
		ft.BytesTransferred,
		ft.Status,
		ft.Error,
		nullTime(ft.StartedAt),
		nullTime(ft.CompletedAt),
	)
	return err
}

func (s *PostgresStore) UpdateFileTransfer(ctx context.Context, ft *FileTransfer) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE file_transfers
		 SET bytes_transferred = $2, status = $3, error = $4,
		     started_at = $5, completed_at = $6, file_size = $7
		 WHERE id = $1`,
		strings.TrimSpace(ft.ID),
		ft.BytesTransferred,
		ft.Status,
		ft.Error,
		nullTime(ft.StartedAt),
		nullTime(ft.CompletedAt),
		ft.FileSize,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) ListActiveFileTransfers(ctx context.Context) ([]FileTransfer, error) {
	rows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM file_transfers WHERE status IN ('pending', 'in_progress') ORDER BY started_at DESC NULLS LAST`, fileTransferColumns),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]FileTransfer, 0, 16)
	for rows.Next() {
		ft, scanErr := scanFileTransfer(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, ft)
	}
	return out, rows.Err()
}
