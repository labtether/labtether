package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/secrets"

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
		`INSERT INTO webhooks (id, name, url, secret, secret_ciphertext, events, enabled, created_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		wh.ID, wh.Name, wh.URL, wh.Secret, wh.SecretCiphertext, eventsJSON, wh.Enabled, wh.CreatedBy, wh.CreatedAt,
	)
	return err
}

func (s *PostgresStore) GetWebhook(ctx context.Context, id string) (webhooks.Webhook, bool, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, name, url, secret, secret_ciphertext, events, enabled, created_by, created_at, last_triggered_at
		 FROM webhooks WHERE id = $1`, id)
	return scanWebhookRow(row)
}

func (s *PostgresStore) ListWebhooks(ctx context.Context) ([]webhooks.Webhook, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, url, secret, secret_ciphertext, events, enabled, created_by, created_at, last_triggered_at
		 FROM webhooks ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []webhooks.Webhook
	for rows.Next() {
		var wh webhooks.Webhook
		var eventsJSON []byte
		if err := rows.Scan(&wh.ID, &wh.Name, &wh.URL, &wh.Secret, &wh.SecretCiphertext, &eventsJSON,
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

func (s *PostgresStore) UpdateWebhook(ctx context.Context, wh webhooks.Webhook) error {
	eventsJSON, err := json.Marshal(wh.Events)
	if err != nil {
		return fmt.Errorf("marshal events: %w", err)
	}
	if wh.Events == nil {
		eventsJSON = []byte("[]")
	}

	tag, err := s.pool.Exec(ctx,
		`UPDATE webhooks
		 SET name = $2,
		     url = $3,
		     secret = $4,
		     secret_ciphertext = $5,
		     events = $6,
		     enabled = $7,
		     created_by = $8,
		     created_at = $9,
		     last_triggered_at = $10
		 WHERE id = $1`,
		wh.ID,
		wh.Name,
		wh.URL,
		wh.Secret,
		wh.SecretCiphertext,
		eventsJSON,
		wh.Enabled,
		wh.CreatedBy,
		wh.CreatedAt,
		wh.LastTriggeredAt,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteWebhook removes a webhook by ID. Returns nil even if the webhook does
// not exist (idempotent delete).
func (s *PostgresStore) DeleteWebhook(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM webhooks WHERE id = $1`, id)
	return err
}

func (s *PostgresStore) MarkWebhookTriggered(ctx context.Context, id string, at time.Time) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE webhooks SET last_triggered_at = $2 WHERE id = $1`,
		id, at.UTC(),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) MigrateLegacyWebhookSecrets(ctx context.Context, manager *secrets.Manager) error {
	if s == nil || s.pool == nil || manager == nil {
		return nil
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, secret
		   FROM webhooks
		  WHERE COALESCE(TRIM(secret), '') <> ''
		    AND COALESCE(TRIM(secret_ciphertext), '') = ''`)
	if err != nil {
		return fmt.Errorf("query legacy webhook secrets: %w", err)
	}
	defer rows.Close()

	type legacyWebhookSecret struct {
		id     string
		secret string
	}
	var legacy []legacyWebhookSecret
	for rows.Next() {
		var item legacyWebhookSecret
		if err := rows.Scan(&item.id, &item.secret); err != nil {
			return fmt.Errorf("scan legacy webhook secret: %w", err)
		}
		legacy = append(legacy, item)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate legacy webhook secrets: %w", err)
	}
	if len(legacy) == 0 {
		return nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin legacy webhook secret migration: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	for _, item := range legacy {
		ciphertext, err := manager.EncryptString(item.secret, item.id)
		if err != nil {
			return fmt.Errorf("encrypt legacy webhook secret %s: %w", item.id, err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE webhooks
			    SET secret_ciphertext = $2,
			        secret = ''
			  WHERE id = $1`,
			item.id, ciphertext,
		); err != nil {
			return fmt.Errorf("persist legacy webhook secret %s: %w", item.id, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit legacy webhook secret migration: %w", err)
	}
	tx = nil
	return nil
}

func scanWebhookRow(row pgx.Row) (webhooks.Webhook, bool, error) {
	var wh webhooks.Webhook
	var eventsJSON []byte
	err := row.Scan(&wh.ID, &wh.Name, &wh.URL, &wh.Secret, &wh.SecretCiphertext, &eventsJSON,
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
