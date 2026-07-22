package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/idgen"
)

func (s *PostgresStore) UpsertAssetHeartbeat(req assets.HeartbeatRequest) (assets.Asset, error) {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return assets.Asset{}, err
	}
	defer tx.Rollback(ctx)

	if err := lockAgentIdentityAsset(ctx, tx, req.AssetID); err != nil {
		return assets.Asset{}, err
	}
	asset, err := upsertAssetHeartbeatTx(ctx, tx, req, time.Now().UTC())
	if err != nil {
		return assets.Asset{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return assets.Asset{}, err
	}
	return asset, nil
}

func upsertAssetHeartbeatTx(ctx context.Context, tx pgx.Tx, req assets.HeartbeatRequest, now time.Time) (assets.Asset, error) {
	status := req.Status
	if status == "" {
		status = "online"
	}
	groupID := strings.TrimSpace(req.GroupID)
	metadata := cloneMetadata(req.Metadata)
	if !req.AllowAgentIdentityRotation {
		delete(metadata, assets.MetadataKeyAgentIdentityVerifiedAt)
		if !req.AllowAgentIdentityTOFU {
			delete(metadata, assets.MetadataKeyAgentDeviceFingerprint)
			delete(metadata, assets.MetadataKeyAgentDeviceKeyAlgorithm)
		}
	}
	metadataPayload, err := marshalStringMap(metadata)
	if err != nil {
		return assets.Asset{}, err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO assets (id, type, name, source, group_id, status, platform, metadata, created_at, updated_at, last_seen_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $9, $9)
		 ON CONFLICT (id) DO UPDATE
		 SET type = EXCLUDED.type,
		     name = COALESCE(NULLIF(BTRIM(assets.metadata->>'name_override'), ''), EXCLUDED.name),
		     source = EXCLUDED.source,
		     group_id = COALESCE(EXCLUDED.group_id, assets.group_id),
		     status = EXCLUDED.status,
		     platform = EXCLUDED.platform,
		     metadata =
		       (CASE
		          WHEN $10::boolean THEN EXCLUDED.metadata
		          ELSE
		            (CASE
		               WHEN NULLIF(BTRIM(assets.metadata->>'agent_device_fingerprint'), '') IS NOT NULL
		                 AND LOWER(COALESCE(NULLIF(BTRIM(EXCLUDED.metadata->>'agent_device_fingerprint'), ''), ''))
		                   <> LOWER(BTRIM(assets.metadata->>'agent_device_fingerprint'))
		               THEN EXCLUDED.metadata - 'agent_device_key_alg'
		               ELSE EXCLUDED.metadata
		             END)
		            || CASE
		                 WHEN NULLIF(BTRIM(assets.metadata->>'agent_device_fingerprint'), '') IS NOT NULL
		                 THEN jsonb_build_object('agent_device_fingerprint', assets.metadata->>'agent_device_fingerprint')
		                 ELSE '{}'::jsonb
		               END
		            || CASE
		                 WHEN NULLIF(BTRIM(assets.metadata->>'agent_device_key_alg'), '') IS NOT NULL
		                 THEN jsonb_build_object('agent_device_key_alg', assets.metadata->>'agent_device_key_alg')
		                 ELSE '{}'::jsonb
		               END
		            || CASE
		                 WHEN NULLIF(BTRIM(assets.metadata->>'agent_identity_verified_at'), '') IS NOT NULL
		                 THEN jsonb_build_object('agent_identity_verified_at', assets.metadata->>'agent_identity_verified_at')
		                 ELSE '{}'::jsonb
		               END
		        END)
		       || CASE
		            WHEN NULLIF(BTRIM(assets.metadata->>'name_override'), '') IS NOT NULL
		            THEN jsonb_build_object('name_override', assets.metadata->>'name_override')
		            ELSE '{}'::jsonb
		          END,
		     updated_at = EXCLUDED.updated_at,
		     last_seen_at = EXCLUDED.last_seen_at`,
		req.AssetID,
		req.Type,
		req.Name,
		req.Source,
		nullIfBlank(groupID),
		status,
		req.Platform,
		metadataPayload,
		now,
		req.AllowAgentIdentityRotation,
	)
	if err != nil {
		return assets.Asset{}, err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO asset_heartbeats (id, asset_id, source, status, metadata, received_at)
		 VALUES ($1, $2, $3, $4, $5::jsonb, $6)`,
		idgen.New("hb"),
		req.AssetID,
		req.Source,
		status,
		metadataPayload,
		now,
	)
	if err != nil {
		return assets.Asset{}, err
	}

	asset, err := scanAsset(tx.QueryRow(ctx,
		`SELECT id, type, name, source, group_id, status, platform, metadata, tags, created_at, updated_at, last_seen_at, host, transport_type
		 FROM assets WHERE id = $1`,
		req.AssetID,
	))
	if err != nil {
		return assets.Asset{}, err
	}

	return asset, nil
}

func (s *PostgresStore) ListAssets() ([]assets.Asset, error) {
	return s.listAssets(context.Background(),
		`SELECT id, type, name, source, group_id, status, platform, metadata, tags, created_at, updated_at, last_seen_at, host, transport_type
		 FROM assets
		 ORDER BY last_seen_at DESC`,
	)
}

func (s *PostgresStore) ListAssetsByGroup(groupID string) ([]assets.Asset, error) {
	return s.listAssets(
		context.Background(),
		`SELECT id, type, name, source, group_id, status, platform, metadata, tags, created_at, updated_at, last_seen_at, host, transport_type
		 FROM assets
		 WHERE group_id = $1
		 ORDER BY last_seen_at DESC`,
		nullIfBlank(strings.TrimSpace(groupID)),
	)
}

func (s *PostgresStore) listAssets(ctx context.Context, query string, args ...any) ([]assets.Asset, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]assets.Asset, 0, 32)
	for rows.Next() {
		asset, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, asset)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) UpdateAsset(id string, req assets.UpdateRequest) (assets.Asset, error) {
	current, ok, err := s.GetAsset(id)
	if err != nil {
		return assets.Asset{}, err
	}
	if !ok {
		return assets.Asset{}, ErrNotFound
	}

	nextName := current.Name
	if req.Name != nil {
		nextName = strings.TrimSpace(*req.Name)
	}

	nextGroupID := strings.TrimSpace(current.GroupID)
	if req.GroupID != nil {
		nextGroupID = strings.TrimSpace(*req.GroupID)
	}

	nextTags := assets.NormalizeTags(current.Tags)
	if req.Tags != nil {
		nextTags = assets.NormalizeTags(*req.Tags)
	}

	nextMetadata := cloneMetadata(current.Metadata)
	if req.Name != nil {
		if nextMetadata == nil {
			nextMetadata = map[string]string{}
		}
		nextMetadata[assets.MetadataKeyNameOverride] = nextName
	}

	tagsPayload, err := marshalStringSlice(nextTags)
	if err != nil {
		return assets.Asset{}, err
	}
	metadataPayload, err := marshalStringMap(nextMetadata)
	if err != nil {
		return assets.Asset{}, err
	}

	updatedAt := time.Now().UTC()
	asset, err := scanAsset(s.pool.QueryRow(context.Background(),
		`UPDATE assets
			   SET name = $2,
			       group_id = $3,
			       updated_at = $4,
			       tags = $5::jsonb,
			       metadata = $6::jsonb
			 WHERE id = $1
			 RETURNING id, type, name, source, group_id, status, platform, metadata, tags, created_at, updated_at, last_seen_at, host, transport_type`,
		id,
		nextName,
		nullIfBlank(nextGroupID),
		updatedAt,
		tagsPayload,
		metadataPayload,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return assets.Asset{}, ErrNotFound
		}
		return assets.Asset{}, err
	}

	return asset, nil
}

func (s *PostgresStore) GetAsset(id string) (assets.Asset, bool, error) {
	asset, err := scanAsset(s.pool.QueryRow(context.Background(),
		`SELECT id, type, name, source, group_id, status, platform, metadata, tags, created_at, updated_at, last_seen_at, host, transport_type
		 FROM assets WHERE id = $1`,
		id,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return assets.Asset{}, false, nil
		}
		return assets.Asset{}, false, err
	}

	return asset, true, nil
}

func (s *PostgresStore) DeleteAsset(id string) error {
	tag, err := s.pool.Exec(context.Background(), `DELETE FROM assets WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
