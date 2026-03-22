package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/idgen"
)

// WebServiceAltURL represents an alternative URL associated with a web service.
type WebServiceAltURL struct {
	ID           string    `json:"id"`
	WebServiceID string    `json:"web_service_id"`
	URL          string    `json:"url"`
	Source       string    `json:"source"`
	CreatedAt    time.Time `json:"created_at"`
}

// WebServiceURLGroupingSetting stores a key/value setting for URL grouping behavior.
type WebServiceURLGroupingSetting struct {
	ID           string    `json:"id"`
	SettingKey   string    `json:"setting_key"`
	SettingValue string    `json:"setting_value"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// WebServiceNeverGroupRule stores a pair of URLs that should never be grouped together.
type WebServiceNeverGroupRule struct {
	ID        string    `json:"id"`
	URLA      string    `json:"url_a"`
	URLB      string    `json:"url_b"`
	CreatedAt time.Time `json:"created_at"`
}

// ListAltURLsByService returns all alternative URLs for a specific web service,
// ordered by creation time ascending.
func (s *PostgresStore) ListAltURLsByService(ctx context.Context, webServiceID string) ([]WebServiceAltURL, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, web_service_id, url, source, created_at
		 FROM web_service_alt_urls
		 WHERE web_service_id = $1
		 ORDER BY created_at ASC`,
		webServiceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]WebServiceAltURL, 0, 16)
	for rows.Next() {
		var item WebServiceAltURL
		if err := rows.Scan(
			&item.ID,
			&item.WebServiceID,
			&item.URL,
			&item.Source,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

// ListAllAltURLs returns all alternative URLs across all services,
// ordered by web_service_id then creation time ascending.
func (s *PostgresStore) ListAllAltURLs(ctx context.Context) ([]WebServiceAltURL, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, web_service_id, url, source, created_at
		 FROM web_service_alt_urls
		 ORDER BY web_service_id, created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]WebServiceAltURL, 0, 64)
	for rows.Next() {
		var item WebServiceAltURL
		if err := rows.Scan(
			&item.ID,
			&item.WebServiceID,
			&item.URL,
			&item.Source,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

// UpsertAltURL inserts a new alternative URL or updates its source if the
// (web_service_id, url) combination already exists.
func (s *PostgresStore) UpsertAltURL(ctx context.Context, webServiceID, url, source string) error {
	id := idgen.New("alturl")
	now := time.Now().UTC()

	_, err := s.pool.Exec(ctx,
		`INSERT INTO web_service_alt_urls (id, web_service_id, url, source, created_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (web_service_id, url) DO UPDATE SET source = EXCLUDED.source`,
		id, webServiceID, url, source, now,
	)
	return err
}

// DeleteAltURL removes an alternative URL by its ID.
func (s *PostgresStore) DeleteAltURL(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM web_service_alt_urls WHERE id = $1`,
		id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAltURLByServiceAndURL removes an alternative URL by its web service ID and URL.
func (s *PostgresStore) DeleteAltURLByServiceAndURL(ctx context.Context, webServiceID, url string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM web_service_alt_urls WHERE web_service_id = $1 AND url = $2`,
		webServiceID, url,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListNeverGroupRules returns all never-group rules, ordered by creation time ascending.
func (s *PostgresStore) ListNeverGroupRules(ctx context.Context) ([]WebServiceNeverGroupRule, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, url_a, url_b, created_at
		 FROM web_service_never_group_rules
		 ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]WebServiceNeverGroupRule, 0, 16)
	for rows.Next() {
		var item WebServiceNeverGroupRule
		if err := rows.Scan(
			&item.ID,
			&item.URLA,
			&item.URLB,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

// UpsertNeverGroupRule inserts a never-group rule for a pair of URLs.
// The URLs are normalized so that the alphabetically smaller URL is always url_a.
// If the pair already exists, the insert is silently ignored.
func (s *PostgresStore) UpsertNeverGroupRule(ctx context.Context, urlA, urlB string) error {
	// Normalize order: url_a < url_b alphabetically.
	if urlA > urlB {
		urlA, urlB = urlB, urlA
	}

	id := idgen.New("ngrule")
	now := time.Now().UTC()

	_, err := s.pool.Exec(ctx,
		`INSERT INTO web_service_never_group_rules (id, url_a, url_b, created_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (url_a, url_b) DO NOTHING`,
		id, urlA, urlB, now,
	)
	return err
}

// DeleteNeverGroupRule removes a never-group rule by its ID.
func (s *PostgresStore) DeleteNeverGroupRule(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM web_service_never_group_rules WHERE id = $1`,
		id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetURLGroupingSetting returns the value for a single grouping setting key.
// Returns empty string and sql.ErrNoRows if the key does not exist.
func (s *PostgresStore) GetURLGroupingSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.pool.QueryRow(ctx,
		`SELECT setting_value FROM web_service_url_grouping_settings WHERE setting_key = $1`,
		key,
	).Scan(&value)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return value, nil
}

// ListURLGroupingSettings returns all URL grouping settings, ordered by setting_key.
func (s *PostgresStore) ListURLGroupingSettings(ctx context.Context) ([]WebServiceURLGroupingSetting, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, setting_key, setting_value, updated_at
		 FROM web_service_url_grouping_settings
		 ORDER BY setting_key`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]WebServiceURLGroupingSetting, 0, 8)
	for rows.Next() {
		var item WebServiceURLGroupingSetting
		if err := rows.Scan(
			&item.ID,
			&item.SettingKey,
			&item.SettingValue,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

// UpsertURLGroupingSetting inserts or updates a URL grouping setting.
func (s *PostgresStore) UpsertURLGroupingSetting(ctx context.Context, key, value string) error {
	id := idgen.New("grpset")
	now := time.Now().UTC()

	_, err := s.pool.Exec(ctx,
		`INSERT INTO web_service_url_grouping_settings (id, setting_key, setting_value, updated_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (setting_key) DO UPDATE SET
			setting_value = EXCLUDED.setting_value,
			updated_at = EXCLUDED.updated_at`,
		id, key, value, now,
	)
	return err
}
