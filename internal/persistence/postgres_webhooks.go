package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/webhooks"
)

func (s *PostgresStore) CreateWebhook(ctx context.Context, wh webhooks.Webhook) error {
	eventsJSON, err := json.Marshal(wh.Events)
	if err != nil {
		return fmt.Errorf("marshal events: %w", err)
	}
	if wh.Events == nil {
		eventsJSON = []byte("[]")
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO webhooks (id, name, url, secret, events, enabled, created_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		wh.ID, wh.Name, wh.URL, wh.Secret, eventsJSON, wh.Enabled, wh.CreatedBy, wh.CreatedAt,
	)
	return err
}

func (s *PostgresStore) GetWebhook(ctx context.Context, id string) (webhooks.Webhook, bool, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, name, url, secret, events, enabled, created_by, created_at, last_triggered_at
		 FROM webhooks WHERE id = $1`, id)
	return scanWebhookRow(row)
}

func (s *PostgresStore) ListWebhooks(ctx context.Context) ([]webhooks.Webhook, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, url, secret, events, enabled, created_by, created_at, last_triggered_at
		 FROM webhooks ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []webhooks.Webhook
	for rows.Next() {
		var wh webhooks.Webhook
		var eventsJSON []byte
		if err := rows.Scan(&wh.ID, &wh.Name, &wh.URL, &wh.Secret, &eventsJSON,
			&wh.Enabled, &wh.CreatedBy, &wh.CreatedAt, &wh.LastTriggeredAt); err != nil {
			return nil, err
		}
		if eventsJSON != nil {
			if err := json.Unmarshal(eventsJSON, &wh.Events); err != nil {
				return nil, fmt.Errorf("corrupt events JSON for webhook %s: %w", wh.ID, err)
			}
		}
		result = append(result, wh)
	}
	return result, rows.Err()
}

func (s *PostgresStore) UpdateWebhook(ctx context.Context, id string, name *string, url *string, events *[]string, enabled *bool) error {
	var setClauses []string
	var args []any
	argIdx := 1

	if name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *name)
		argIdx++
	}
	if url != nil {
		setClauses = append(setClauses, fmt.Sprintf("url = $%d", argIdx))
		args = append(args, *url)
		argIdx++
	}
	if events != nil {
		eventsJSON, err := json.Marshal(*events)
		if err != nil {
			return fmt.Errorf("marshal events: %w", err)
		}
		setClauses = append(setClauses, fmt.Sprintf("events = $%d", argIdx))
		args = append(args, eventsJSON)
		argIdx++
	}
	if enabled != nil {
		setClauses = append(setClauses, fmt.Sprintf("enabled = $%d", argIdx))
		args = append(args, *enabled)
		argIdx++
	}

	if len(setClauses) == 0 {
		return nil
	}

	query := fmt.Sprintf("UPDATE webhooks SET %s WHERE id = $%d",
		strings.Join(setClauses, ", "), argIdx)
	args = append(args, id)
	_, err := s.pool.Exec(ctx, query, args...)
	return err
}

// DeleteWebhook removes a webhook by ID. Returns nil even if the webhook does
// not exist (idempotent delete).
func (s *PostgresStore) DeleteWebhook(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM webhooks WHERE id = $1`, id)
	return err
}

func scanWebhookRow(row pgx.Row) (webhooks.Webhook, bool, error) {
	var wh webhooks.Webhook
	var eventsJSON []byte
	err := row.Scan(&wh.ID, &wh.Name, &wh.URL, &wh.Secret, &eventsJSON,
		&wh.Enabled, &wh.CreatedBy, &wh.CreatedAt, &wh.LastTriggeredAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return webhooks.Webhook{}, false, nil
		}
		return webhooks.Webhook{}, false, err
	}
	if eventsJSON != nil {
		if err := json.Unmarshal(eventsJSON, &wh.Events); err != nil {
			return webhooks.Webhook{}, false, fmt.Errorf("corrupt events JSON for webhook %s: %w", wh.ID, err)
		}
	}
	return wh, true, nil
}
