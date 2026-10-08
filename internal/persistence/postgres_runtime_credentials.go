package persistence

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/idgen"
	"strings"
	"time"
)

const credentialProfileCardinalityLockID int64 = 0x4c5443524544

type credentialProfileQueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *PostgresStore) CreateCredentialProfile(profile credentials.Profile) (credentials.Profile, error) {
	return createCredentialProfile(context.Background(), s.pool, profile)
}

func (s *PostgresStore) CreateCredentialProfileBounded(
	profile credentials.Profile,
	ownerID string,
	perOwnerLimit, globalLimit int,
) (credentials.Profile, error) {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return credentials.Profile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, credentialProfileCardinalityLockID); err != nil {
		return credentials.Profile{}, err
	}
	if globalLimit > 0 {
		var total int
		if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM credential_profiles`).Scan(&total); err != nil {
			return credentials.Profile{}, err
		}
		if total >= globalLimit {
			return credentials.Profile{}, ErrCredentialProfileGlobalLimit
		}
	}

	profile.CreatedBy = strings.TrimSpace(ownerID)
	if profile.CreatedBy != "" && perOwnerLimit > 0 {
		var ownerTotal int
		if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM credential_profiles WHERE created_by = $1`, profile.CreatedBy).Scan(&ownerTotal); err != nil {
			return credentials.Profile{}, err
		}
		if ownerTotal >= perOwnerLimit {
			return credentials.Profile{}, ErrCredentialProfileOwnerLimit
		}
	}

	created, err := createCredentialProfile(ctx, tx, profile)
	if err != nil {
		return credentials.Profile{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return credentials.Profile{}, err
	}
	return created, nil
}

func createCredentialProfile(ctx context.Context, q credentialProfileQueryRower, profile credentials.Profile) (credentials.Profile, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(profile.ID) == "" {
		profile.ID = idgen.New("cred")
	}
	status := strings.TrimSpace(profile.Status)
	if status == "" {
		status = "active"
	}
	metadataPayload, err := marshalStringMap(profile.Metadata)
	if err != nil {
		return credentials.Profile{}, err
	}

	created, err := scanCredentialProfile(q.QueryRow(ctx,
		`INSERT INTO credential_profiles (
			id,
			name,
			kind,
			username,
			description,
			status,
			created_by,
			metadata,
			secret_ciphertext,
			passphrase_ciphertext,
			created_at,
			updated_at,
			rotated_at,
			expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10, $11, $11, $11, $12)
		RETURNING
			id,
			name,
			kind,
			username,
			description,
			status,
			created_by,
			metadata,
			secret_ciphertext,
			passphrase_ciphertext,
			created_at,
			updated_at,
			rotated_at,
			last_used_at,
			expires_at`,
		profile.ID,
		strings.TrimSpace(profile.Name),
		strings.TrimSpace(profile.Kind),
		nullIfBlank(profile.Username),
		nullIfBlank(profile.Description),
		status,
		strings.TrimSpace(profile.CreatedBy),
		metadataPayload,
		profile.SecretCiphertext,
		nullIfEmptyExact(profile.PassphraseCiphertext),
		now,
		nullTime(profile.ExpiresAt),
	))
	if err != nil {
		return credentials.Profile{}, err
	}
	return created, nil
}

func (s *PostgresStore) UpdateCredentialProfile(profile credentials.Profile) (credentials.Profile, error) {
	return updateCredentialProfile(context.Background(), s.pool, profile)
}

func updateCredentialProfile(ctx context.Context, q credentialProfileQueryRower, profile credentials.Profile) (credentials.Profile, error) {
	now := time.Now().UTC()
	status := strings.TrimSpace(profile.Status)
	if status == "" {
		status = "active"
	}
	metadataPayload, err := marshalStringMap(profile.Metadata)
	if err != nil {
		return credentials.Profile{}, err
	}

	updated, err := scanCredentialProfile(q.QueryRow(ctx,
		`UPDATE credential_profiles
		 SET name = $2,
		     kind = $3,
		     username = $4,
		     description = $5,
		     status = $6,
		     metadata = $7::jsonb,
		     secret_ciphertext = $8,
		     passphrase_ciphertext = $9,
		     updated_at = $10,
		     rotated_at = $11,
		     expires_at = $12
		 WHERE id = $1
		 RETURNING
			id,
			name,
			kind,
			username,
				description,
				status,
				created_by,
				metadata,
			secret_ciphertext,
			passphrase_ciphertext,
			created_at,
			updated_at,
			rotated_at,
			last_used_at,
			expires_at`,
		strings.TrimSpace(profile.ID),
		strings.TrimSpace(profile.Name),
		strings.TrimSpace(profile.Kind),
		nullIfBlank(profile.Username),
		nullIfBlank(profile.Description),
		status,
		metadataPayload,
		profile.SecretCiphertext,
		nullIfEmptyExact(profile.PassphraseCiphertext),
		now,
		nullTime(profile.RotatedAt),
		nullTime(profile.ExpiresAt),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return credentials.Profile{}, errors.New("credential profile not found")
		}
		return credentials.Profile{}, err
	}
	return updated, nil
}

func (s *PostgresStore) UpdateCredentialProfileSecret(id, secretCiphertext, passphraseCiphertext string, expiresAt *time.Time) (credentials.Profile, error) {
	now := time.Now().UTC()
	updated, err := scanCredentialProfile(s.pool.QueryRow(context.Background(),
		`UPDATE credential_profiles
		 SET secret_ciphertext = $2,
		     passphrase_ciphertext = $3,
		     rotated_at = $4,
		     updated_at = $4,
		     expires_at = $5
		 WHERE id = $1
		 RETURNING
			id,
			name,
			kind,
			username,
				description,
				status,
				created_by,
				metadata,
			secret_ciphertext,
			passphrase_ciphertext,
			created_at,
			updated_at,
			rotated_at,
			last_used_at,
			expires_at`,
		strings.TrimSpace(id),
		secretCiphertext,
		nullIfEmptyExact(passphraseCiphertext),
		now,
		nullTime(expiresAt),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return credentials.Profile{}, errors.New("credential profile not found")
		}
		return credentials.Profile{}, err
	}
	return updated, nil
}

func (s *PostgresStore) GetCredentialProfile(id string) (credentials.Profile, bool, error) {
	profile, err := scanCredentialProfile(s.pool.QueryRow(context.Background(),
		`SELECT
			id,
			name,
			kind,
			username,
				description,
				status,
				created_by,
				metadata,
			secret_ciphertext,
			passphrase_ciphertext,
			created_at,
			updated_at,
			rotated_at,
			last_used_at,
			expires_at
		 FROM credential_profiles
		 WHERE id = $1`,
		strings.TrimSpace(id),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return credentials.Profile{}, false, nil
		}
		return credentials.Profile{}, false, err
	}
	return profile, true, nil
}

func (s *PostgresStore) ListCredentialProfiles(limit int) ([]credentials.Profile, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.pool.Query(context.Background(),
		`SELECT
			id,
			name,
			kind,
			username,
				description,
				status,
				created_by,
				metadata,
			secret_ciphertext,
			passphrase_ciphertext,
			created_at,
			updated_at,
			rotated_at,
			last_used_at,
			expires_at
		 FROM credential_profiles
		 ORDER BY updated_at DESC
		 LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]credentials.Profile, 0)
	for rows.Next() {
		profile, scanErr := scanCredentialProfile(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, profile)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) MarkCredentialProfileUsed(id string, usedAt time.Time) error {
	tag, err := s.pool.Exec(context.Background(),
		`UPDATE credential_profiles
		 SET last_used_at = $2,
		     updated_at = GREATEST(updated_at, $2)
		 WHERE id = $1`,
		strings.TrimSpace(id),
		usedAt.UTC(),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("credential profile not found")
	}
	return nil
}

func (s *PostgresStore) DeleteCredentialProfile(id string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM credential_profiles WHERE id = $1`,
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

func (s *PostgresStore) DeleteCredentialProfileIfUnreferenced(id string) (CredentialProfileReferenceSummary, error) {
	ctx := context.Background()
	id = strings.TrimSpace(id)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CredentialProfileReferenceSummary{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var kind string
	if err = tx.QueryRow(ctx, `SELECT kind FROM credential_profiles WHERE id = $1 FOR UPDATE`, id).Scan(&kind); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CredentialProfileReferenceSummary{}, ErrNotFound
		}
		return CredentialProfileReferenceSummary{}, err
	}
	if strings.TrimSpace(kind) == credentials.KindHubSSHIdentity {
		return CredentialProfileReferenceSummary{}, ErrCredentialProfileProtected
	}

	checks := []struct {
		resource string
		query    string
	}{
		{"asset_terminal_configs", `SELECT COUNT(*) FROM asset_terminal_configs WHERE credential_profile_id = $1`},
		{"asset_desktop_configs", `SELECT COUNT(*) FROM asset_desktop_configs WHERE credential_profile_id = $1`},
		{"asset_protocol_configs", `SELECT COUNT(*) FROM asset_protocol_configs WHERE credential_profile_id = $1`},
		{"terminal_session_bookmarks", `SELECT COUNT(*) FROM terminal_session_bookmarks WHERE credential_profile_id = $1`},
		{"group_jump_chains", `SELECT COUNT(*) FROM groups WHERE jump_chain @> jsonb_build_object('hops', jsonb_build_array(jsonb_build_object('credential_profile_id', $1::text)))`},
		{"hub_collectors", `SELECT COUNT(*) FROM hub_collectors WHERE config->>'credential_id' = $1`},
		{"remote_bookmarks", `SELECT COUNT(*) FROM remote_bookmarks WHERE credential_id = $1`},
		{"file_connections", `SELECT COUNT(*) FROM file_connections WHERE credential_id = $1`},
	}
	summary := CredentialProfileReferenceSummary{References: []CredentialProfileReference{}}
	for _, check := range checks {
		count := 0
		if err = tx.QueryRow(ctx, check.query, id).Scan(&count); err != nil {
			return CredentialProfileReferenceSummary{}, err
		}
		if count > 0 {
			summary.References = append(summary.References, CredentialProfileReference{Resource: check.resource, Count: count})
			summary.Total += count
		}
	}
	if summary.Total > 0 {
		return summary, ErrCredentialProfileInUse
	}

	tag, err := tx.Exec(ctx, `DELETE FROM credential_profiles WHERE id = $1`, id)
	if err != nil {
		return CredentialProfileReferenceSummary{}, err
	}
	if tag.RowsAffected() == 0 {
		return CredentialProfileReferenceSummary{}, ErrNotFound
	}
	if err = tx.Commit(ctx); err != nil {
		return CredentialProfileReferenceSummary{}, err
	}
	return summary, nil
}

func nullIfEmptyExact(value string) any {
	if value == "" {
		return nil
	}
	return value
}
