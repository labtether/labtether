package persistence

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/idgen"
)

func (s *PostgresStore) ListManualWebServices(hostAssetID string) ([]WebServiceManual, error) {
	ctx := context.Background()
	host := strings.TrimSpace(hostAssetID)

	query := `SELECT id, host_asset_id, name, category, url, icon_key, metadata, created_at, updated_at
		FROM web_services_manual`
	args := make([]any, 0, 1)
	if host != "" {
		query += ` WHERE host_asset_id = $1`
		args = append(args, host)
	}
	query += ` ORDER BY updated_at DESC, created_at DESC`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]WebServiceManual, 0, 32)
	for rows.Next() {
		var item WebServiceManual
		var hostID sql.NullString
		var metadata []byte
		if err := rows.Scan(
			&item.ID,
			&hostID,
			&item.Name,
			&item.Category,
			&item.URL,
			&item.IconKey,
			&metadata,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.HostAssetID = hostID.String
		item.Metadata = unmarshalStringMap(metadata)
		out = append(out, item)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) GetManualWebService(id string) (WebServiceManual, bool, error) {
	ctx := context.Background()
	key := strings.TrimSpace(id)
	if key == "" {
		return WebServiceManual{}, false, nil
	}

	var item WebServiceManual
	var hostID sql.NullString
	var metadata []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, host_asset_id, name, category, url, icon_key, metadata, created_at, updated_at
		 FROM web_services_manual
		 WHERE id = $1`,
		key,
	).Scan(
		&item.ID,
		&hostID,
		&item.Name,
		&item.Category,
		&item.URL,
		&item.IconKey,
		&metadata,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	item.HostAssetID = hostID.String
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WebServiceManual{}, false, nil
		}
		return WebServiceManual{}, false, err
	}
	item.Metadata = unmarshalStringMap(metadata)
	return item, true, nil
}

func (s *PostgresStore) SaveManualWebService(service WebServiceManual) (WebServiceManual, error) {
	ctx := context.Background()
	now := time.Now().UTC()

	item := WebServiceManual{
		ID:          strings.TrimSpace(service.ID),
		HostAssetID: strings.TrimSpace(service.HostAssetID),
		Name:        strings.TrimSpace(service.Name),
		Category:    strings.TrimSpace(service.Category),
		URL:         strings.TrimSpace(service.URL),
		IconKey:     strings.TrimSpace(service.IconKey),
		Metadata:    cloneMetadata(service.Metadata),
		UpdatedAt:   now,
	}
	if item.ID == "" {
		item.ID = idgen.New("wsvc")
	}

	metadataJSON, err := marshalStringMap(item.Metadata)
	if err != nil {
		return WebServiceManual{}, err
	}

	hostParam := sql.NullString{String: item.HostAssetID, Valid: item.HostAssetID != ""}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO web_services_manual (
			id, host_asset_id, name, category, url, icon_key, metadata, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9
		)
		ON CONFLICT (id) DO UPDATE SET
			host_asset_id = EXCLUDED.host_asset_id,
			name = EXCLUDED.name,
			category = EXCLUDED.category,
			url = EXCLUDED.url,
			icon_key = EXCLUDED.icon_key,
			metadata = EXCLUDED.metadata,
			updated_at = EXCLUDED.updated_at`,
		item.ID,
		hostParam,
		item.Name,
		item.Category,
		item.URL,
		item.IconKey,
		metadataJSON,
		now,
		now,
	)
	if err != nil {
		return WebServiceManual{}, err
	}

	saved, _, err := s.GetManualWebService(item.ID)
	if err != nil {
		return WebServiceManual{}, err
	}
	return saved, nil
}

func (s *PostgresStore) DeleteManualWebService(id string) error {
	ctx := context.Background()
	key := strings.TrimSpace(id)
	if key == "" {
		return ErrNotFound
	}

	tag, err := s.pool.Exec(ctx, `DELETE FROM web_services_manual WHERE id = $1`, key)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) PromoteManualServicesToStandalone(hostAssetID string) error {
	_, err := s.pool.Exec(context.Background(),
		`UPDATE web_services_manual SET host_asset_id = NULL, updated_at = NOW() WHERE host_asset_id = $1`,
		hostAssetID)
	return err
}

func (s *PostgresStore) ListWebServiceOverrides(hostAssetID string) ([]WebServiceOverride, error) {
	ctx := context.Background()
	host := strings.TrimSpace(hostAssetID)

	query := `SELECT host_asset_id, service_id, name_override, category_override, url_override, icon_key_override, tags_override, hidden, updated_at
		FROM web_service_overrides`
	args := make([]any, 0, 1)
	if host != "" {
		query += ` WHERE host_asset_id = $1`
		args = append(args, host)
	}
	query += ` ORDER BY updated_at DESC`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]WebServiceOverride, 0, 64)
	for rows.Next() {
		var item WebServiceOverride
		if err := rows.Scan(
			&item.HostAssetID,
			&item.ServiceID,
			&item.NameOverride,
			&item.CategoryOverride,
			&item.URLOverride,
			&item.IconKeyOverride,
			&item.TagsOverride,
			&item.Hidden,
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

func (s *PostgresStore) SaveWebServiceOverride(override WebServiceOverride) (WebServiceOverride, error) {
	ctx := context.Background()
	now := time.Now().UTC()

	item := WebServiceOverride{
		HostAssetID:      strings.TrimSpace(override.HostAssetID),
		ServiceID:        strings.TrimSpace(override.ServiceID),
		NameOverride:     strings.TrimSpace(override.NameOverride),
		CategoryOverride: strings.TrimSpace(override.CategoryOverride),
		URLOverride:      strings.TrimSpace(override.URLOverride),
		IconKeyOverride:  strings.TrimSpace(override.IconKeyOverride),
		TagsOverride:     strings.TrimSpace(override.TagsOverride),
		Hidden:           override.Hidden,
		UpdatedAt:        now,
	}

	_, err := s.pool.Exec(ctx,
		`INSERT INTO web_service_overrides (
			host_asset_id, service_id, name_override, category_override, url_override, icon_key_override, tags_override, hidden, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		)
		ON CONFLICT (host_asset_id, service_id) DO UPDATE SET
			name_override = EXCLUDED.name_override,
			category_override = EXCLUDED.category_override,
			url_override = EXCLUDED.url_override,
			icon_key_override = EXCLUDED.icon_key_override,
			tags_override = EXCLUDED.tags_override,
			hidden = EXCLUDED.hidden,
			updated_at = EXCLUDED.updated_at`,
		item.HostAssetID,
		item.ServiceID,
		item.NameOverride,
		item.CategoryOverride,
		item.URLOverride,
		item.IconKeyOverride,
		item.TagsOverride,
		item.Hidden,
		item.UpdatedAt,
	)
	if err != nil {
		return WebServiceOverride{}, err
	}

	return item, nil
}

func (s *PostgresStore) DeleteWebServiceOverride(hostAssetID, serviceID string) error {
	ctx := context.Background()
	host := strings.TrimSpace(hostAssetID)
	svcID := strings.TrimSpace(serviceID)
	if host == "" || svcID == "" {
		return ErrNotFound
	}

	tag, err := s.pool.Exec(ctx,
		`DELETE FROM web_service_overrides WHERE host_asset_id = $1 AND service_id = $2`,
		host,
		svcID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
