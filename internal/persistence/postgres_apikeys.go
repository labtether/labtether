package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/apikeys"
)

func (s *PostgresStore) CreateAPIKey(ctx context.Context, key apikeys.APIKey) error {
	scopesJSON, err := json.Marshal(key.Scopes)
	if err != nil {
		return fmt.Errorf("marshal scopes: %w", err)
	}
	assetsJSON, err := json.Marshal(key.AllowedAssets)
	if err != nil {
		return fmt.Errorf("marshal allowed_assets: %w", err)
	}
	if key.AllowedAssets == nil {
		assetsJSON = []byte("[]")
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO api_keys (id, name, prefix, secret_hash, role, scopes, allowed_assets, expires_at, created_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		key.ID, key.Name, key.Prefix, key.SecretHash, key.Role,
		scopesJSON, assetsJSON, key.ExpiresAt, key.CreatedBy, key.CreatedAt,
	)
	return err
}

func (s *PostgresStore) LookupAPIKeyByHash(ctx context.Context, secretHash string) (apikeys.APIKey, bool, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, name, prefix, secret_hash, role, scopes, allowed_assets, expires_at, created_by, created_at, last_used_at
		 FROM api_keys WHERE secret_hash = $1`, secretHash)
	return scanAPIKeyRow(row)
}

func (s *PostgresStore) GetAPIKey(ctx context.Context, id string) (apikeys.APIKey, bool, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, name, prefix, secret_hash, role, scopes, allowed_assets, expires_at, created_by, created_at, last_used_at
		 FROM api_keys WHERE id = $1`, id)
	return scanAPIKeyRow(row)
}

func (s *PostgresStore) ListAPIKeys(ctx context.Context) ([]apikeys.APIKey, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, prefix, secret_hash, role, scopes, allowed_assets, expires_at, created_by, created_at, last_used_at
		 FROM api_keys ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []apikeys.APIKey
	for rows.Next() {
		var k apikeys.APIKey
		var scopesJSON, assetsJSON []byte
		err := rows.Scan(&k.ID, &k.Name, &k.Prefix, &k.SecretHash, &k.Role,
			&scopesJSON, &assetsJSON, &k.ExpiresAt, &k.CreatedBy, &k.CreatedAt, &k.LastUsedAt)
		if err != nil {
			return nil, err
		}
		if scopesJSON != nil {
			if err := json.Unmarshal(scopesJSON, &k.Scopes); err != nil {
				return nil, fmt.Errorf("corrupt scopes JSON for key %s: %w", k.ID, err)
			}
		}
		if assetsJSON != nil {
			if err := json.Unmarshal(assetsJSON, &k.AllowedAssets); err != nil {
				return nil, fmt.Errorf("corrupt allowed_assets JSON for key %s: %w", k.ID, err)
			}
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *PostgresStore) UpdateAPIKey(ctx context.Context, id string, name *string, scopes *[]string, allowedAssets *[]string, expiresAt *time.Time) error {
	var setClauses []string
	var args []any
	argIdx := 1

	if name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *name)
		argIdx++
	}
	if scopes != nil {
		scopesJSON, err := json.Marshal(*scopes)
		if err != nil {
			return fmt.Errorf("marshal scopes: %w", err)
		}
		setClauses = append(setClauses, fmt.Sprintf("scopes = $%d", argIdx))
		args = append(args, scopesJSON)
		argIdx++
	}
	if allowedAssets != nil {
		assetsJSON, err := json.Marshal(*allowedAssets)
		if err != nil {
			return fmt.Errorf("marshal allowed_assets: %w", err)
		}
		setClauses = append(setClauses, fmt.Sprintf("allowed_assets = $%d", argIdx))
		args = append(args, assetsJSON)
		argIdx++
	}
	if expiresAt != nil {
		setClauses = append(setClauses, fmt.Sprintf("expires_at = $%d", argIdx))
		args = append(args, *expiresAt)
		argIdx++
	}

	if len(setClauses) == 0 {
		return nil
	}

	query := fmt.Sprintf("UPDATE api_keys SET %s WHERE id = $%d",
		strings.Join(setClauses, ", "), argIdx)
	args = append(args, id)
	_, err := s.pool.Exec(ctx, query, args...)
	return err
}

// DeleteAPIKey removes a key by ID. Returns nil even if the key does not exist
// (idempotent delete).
func (s *PostgresStore) DeleteAPIKey(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM api_keys WHERE id = $1`, id)
	return err
}

func (s *PostgresStore) TouchAPIKeyLastUsed(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE api_keys SET last_used_at = $1 WHERE id = $2`,
		time.Now().UTC(), id)
	return err
}

func scanAPIKeyRow(row pgx.Row) (apikeys.APIKey, bool, error) {
	var k apikeys.APIKey
	var scopesJSON, assetsJSON []byte
	err := row.Scan(&k.ID, &k.Name, &k.Prefix, &k.SecretHash, &k.Role,
		&scopesJSON, &assetsJSON, &k.ExpiresAt, &k.CreatedBy, &k.CreatedAt, &k.LastUsedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apikeys.APIKey{}, false, nil
		}
		return apikeys.APIKey{}, false, err
	}
	if scopesJSON != nil {
		if err := json.Unmarshal(scopesJSON, &k.Scopes); err != nil {
			return apikeys.APIKey{}, false, fmt.Errorf("corrupt scopes JSON for key %s: %w", k.ID, err)
		}
	}
	if assetsJSON != nil {
		if err := json.Unmarshal(assetsJSON, &k.AllowedAssets); err != nil {
			return apikeys.APIKey{}, false, fmt.Errorf("corrupt allowed_assets JSON for key %s: %w", k.ID, err)
		}
	}
	return k, true, nil
}
