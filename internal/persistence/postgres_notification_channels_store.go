package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/notifications"
)

func (s *PostgresStore) CreateNotificationChannel(req notifications.CreateChannelRequest) (notifications.Channel, error) {
	now := time.Now().UTC()
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	configPayload, err := marshalAnyMap(req.Config)
	if err != nil {
		return notifications.Channel{}, err
	}

	return scanNotificationChannel(s.pool.QueryRow(context.Background(),
		`INSERT INTO notification_channels (id, name, type, config, enabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4::jsonb, $5, $6, $6)
		 RETURNING id, name, type, config, enabled, created_at, updated_at`,
		idgen.New("nch"),
		strings.TrimSpace(req.Name),
		notifications.NormalizeChannelType(req.Type),
		configPayload,
		enabled,
		now,
	))
}

func (s *PostgresStore) GetNotificationChannel(id string) (notifications.Channel, bool, error) {
	ch, err := scanNotificationChannel(s.pool.QueryRow(context.Background(),
		`SELECT id, name, type, config, enabled, created_at, updated_at
		 FROM notification_channels WHERE id = $1`,
		strings.TrimSpace(id),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return notifications.Channel{}, false, nil
		}
		return notifications.Channel{}, false, err
	}
	return ch, true, nil
}

func (s *PostgresStore) ListNotificationChannels(limit int) ([]notifications.Channel, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.pool.Query(context.Background(),
		`SELECT id, name, type, config, enabled, created_at, updated_at
		 FROM notification_channels ORDER BY updated_at DESC LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]notifications.Channel, 0)
	for rows.Next() {
		ch, scanErr := scanNotificationChannel(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

func (s *PostgresStore) UpdateNotificationChannel(id string, req notifications.UpdateChannelRequest) (notifications.Channel, error) {
	existing, ok, err := s.GetNotificationChannel(id)
	if err != nil {
		return notifications.Channel{}, err
	}
	if !ok {
		return notifications.Channel{}, notifications.ErrChannelNotFound
	}

	if req.Name != nil {
		existing.Name = strings.TrimSpace(*req.Name)
	}
	if req.Config != nil {
		existing.Config = *req.Config
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}

	configPayload, err := marshalAnyMap(existing.Config)
	if err != nil {
		return notifications.Channel{}, err
	}

	now := time.Now().UTC()
	return scanNotificationChannel(s.pool.QueryRow(context.Background(),
		`UPDATE notification_channels
		 SET name = $2, config = $3::jsonb, enabled = $4, updated_at = $5
		 WHERE id = $1
		 RETURNING id, name, type, config, enabled, created_at, updated_at`,
		existing.ID, existing.Name, configPayload, existing.Enabled, now,
	))
}

func (s *PostgresStore) DeleteNotificationChannel(id string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM notification_channels WHERE id = $1`,
		strings.TrimSpace(id),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return notifications.ErrChannelNotFound
	}
	return nil
}
