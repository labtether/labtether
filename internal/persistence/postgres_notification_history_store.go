package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/notifications"
)

func (s *PostgresStore) CreateNotificationRecord(req notifications.CreateRecordRequest) (notifications.Record, error) {
	now := time.Now().UTC()
	status := notifications.NormalizeRecordStatus(req.Status)
	if status == "" {
		status = notifications.RecordStatusPending
	}

	var sentAt any
	if status == notifications.RecordStatusSent {
		sentAt = now
	}
	payloadValue, err := marshalAnyMap(req.Payload)
	if err != nil {
		return notifications.Record{}, err
	}

	return scanNotificationRecord(s.pool.QueryRow(context.Background(),
		`INSERT INTO notification_history (
			id, channel_id, alert_instance_id, route_id, payload, status, sent_at, error,
			retry_count, max_retries, next_retry_at, created_at
		)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, channel_id, alert_instance_id, route_id, payload, status, sent_at, error,
			retry_count, max_retries, next_retry_at, created_at`,
		idgen.New("notif"),
		strings.TrimSpace(req.ChannelID),
		nullIfBlank(req.AlertInstanceID),
		nullIfBlank(req.RouteID),
		payloadValue,
		status,
		sentAt,
		strings.TrimSpace(req.Error),
		0,
		notifications.DefaultMaxRetries,
		nil,
		now,
	))
}

func (s *PostgresStore) ListNotificationHistory(limit int, channelID string) ([]notifications.Record, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	where := make([]string, 0, 1)
	args := make([]any, 0, 2)
	next := 1

	if channelID = strings.TrimSpace(channelID); channelID != "" {
		where = append(where, fmt.Sprintf("channel_id = $%d", next))
		args = append(args, channelID)
		next++
	}

	sql := `SELECT id, channel_id, alert_instance_id, route_id, payload, status, sent_at, error,
			retry_count, max_retries, next_retry_at, created_at
			FROM notification_history`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", next)
	args = append(args, limit)

	rows, err := s.pool.Query(context.Background(), sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]notifications.Record, 0)
	for rows.Next() {
		rec, scanErr := scanNotificationRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// ListPendingRetries returns failed notification records whose retry_count has
// not yet reached max_retries and whose next_retry_at is at or before now.
func (s *PostgresStore) ListPendingRetries(ctx context.Context, now time.Time, limit int) ([]notifications.Record, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, channel_id, alert_instance_id, route_id, payload, status, sent_at, error,
			retry_count, max_retries, next_retry_at, created_at
		FROM notification_history
		WHERE status = 'failed' AND retry_count < max_retries AND next_retry_at <= $1
		ORDER BY next_retry_at ASC LIMIT $2`,
		now, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]notifications.Record, 0)
	for rows.Next() {
		rec, scanErr := scanNotificationRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// UpdateRetryState updates the retry_count, next_retry_at, and status columns
// for the notification record with the given id.
func (s *PostgresStore) UpdateRetryState(ctx context.Context, id string, retryCount int, nextRetryAt *time.Time, status, errorMessage string) error {
	status = notifications.NormalizeRecordStatus(status)
	if status == "" {
		status = notifications.RecordStatusFailed
	}
	var sentAt any
	var errorValue any
	if status == notifications.RecordStatusSent {
		sentAt = time.Now().UTC()
	} else {
		errorValue = nullIfBlank(strings.TrimSpace(errorMessage))
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE notification_history
		SET retry_count = $1, next_retry_at = $2, status = $3, error = $4, sent_at = $5
		WHERE id = $6`,
		retryCount, nextRetryAt, status, errorValue, sentAt, strings.TrimSpace(id),
	)
	return err
}
