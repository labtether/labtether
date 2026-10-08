package persistence

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/logs"
	"strings"
	"time"
)

func (s *PostgresStore) SaveView(actorID string, req logs.SavedViewRequest) (logs.SavedView, error) {
	now := time.Now().UTC()
	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = idgen.New("view")
	}
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		actorID = "system"
	}

	row := s.pool.QueryRow(context.Background(),
		`INSERT INTO saved_log_views (id, owner_id, name, asset_id, source, level, search, window_value, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
		 RETURNING id, name, asset_id, source, level, search, window_value, created_at, updated_at`,
		id,
		actorID,
		strings.TrimSpace(req.Name),
		nullIfBlank(req.AssetID),
		nullIfBlank(req.Source),
		nullIfBlank(strings.ToLower(req.Level)),
		nullIfBlank(req.Search),
		nullIfBlank(req.Window),
		now,
	)

	view := logs.SavedView{}
	var assetID *string
	var source *string
	var level *string
	var search *string
	var window *string
	if err := row.Scan(
		&view.ID,
		&view.Name,
		&assetID,
		&source,
		&level,
		&search,
		&window,
		&view.CreatedAt,
		&view.UpdatedAt,
	); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return logs.SavedView{}, ErrAlreadyExists
		}
		return logs.SavedView{}, err
	}
	if assetID != nil {
		view.AssetID = *assetID
	}
	if source != nil {
		view.Source = *source
	}
	if level != nil {
		view.Level = *level
	}
	if search != nil {
		view.Search = *search
	}
	if window != nil {
		view.Window = *window
	}

	return view, nil
}

func (s *PostgresStore) ListViews(actorID string, limit int) ([]logs.SavedView, error) {
	if limit <= 0 {
		limit = 50
	}
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		actorID = "system"
	}

	rows, err := s.pool.Query(context.Background(),
		`SELECT id, name, asset_id, source, level, search, window_value, created_at, updated_at
		 FROM saved_log_views
		 WHERE owner_id = $1
		 ORDER BY updated_at DESC
		 LIMIT $2`,
		actorID,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]logs.SavedView, 0)
	for rows.Next() {
		view := logs.SavedView{}
		var assetID *string
		var source *string
		var level *string
		var search *string
		var window *string
		if err := rows.Scan(
			&view.ID,
			&view.Name,
			&assetID,
			&source,
			&level,
			&search,
			&window,
			&view.CreatedAt,
			&view.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if assetID != nil {
			view.AssetID = *assetID
		}
		if source != nil {
			view.Source = *source
		}
		if level != nil {
			view.Level = *level
		}
		if search != nil {
			view.Search = *search
		}
		if window != nil {
			view.Window = *window
		}
		out = append(out, view)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) GetView(actorID, id string) (logs.SavedView, bool, error) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		actorID = "system"
	}
	row := s.pool.QueryRow(context.Background(),
		`SELECT id, name, asset_id, source, level, search, window_value, created_at, updated_at
		 FROM saved_log_views
		 WHERE owner_id = $1 AND id = $2`,
		actorID,
		strings.TrimSpace(id),
	)

	view := logs.SavedView{}
	var assetID *string
	var source *string
	var level *string
	var search *string
	var window *string
	if err := row.Scan(
		&view.ID,
		&view.Name,
		&assetID,
		&source,
		&level,
		&search,
		&window,
		&view.CreatedAt,
		&view.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return logs.SavedView{}, false, nil
		}
		return logs.SavedView{}, false, err
	}
	if assetID != nil {
		view.AssetID = *assetID
	}
	if source != nil {
		view.Source = *source
	}
	if level != nil {
		view.Level = *level
	}
	if search != nil {
		view.Search = *search
	}
	if window != nil {
		view.Window = *window
	}
	return view, true, nil
}

func (s *PostgresStore) UpdateView(actorID, id string, req logs.SavedViewRequest) (logs.SavedView, error) {
	now := time.Now().UTC()
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		actorID = "system"
	}
	id = strings.TrimSpace(id)

	row := s.pool.QueryRow(context.Background(),
		`UPDATE saved_log_views
		 SET name = $3, asset_id = $4, source = $5, level = $6, search = $7, window_value = $8, updated_at = $9
		 WHERE owner_id = $1 AND id = $2
		 RETURNING id, name, asset_id, source, level, search, window_value, created_at, updated_at`,
		actorID,
		id,
		strings.TrimSpace(req.Name),
		nullIfBlank(req.AssetID),
		nullIfBlank(req.Source),
		nullIfBlank(strings.ToLower(req.Level)),
		nullIfBlank(req.Search),
		nullIfBlank(req.Window),
		now,
	)

	view := logs.SavedView{}
	var viewAssetID *string
	var viewSource *string
	var viewLevel *string
	var viewSearch *string
	var viewWindow *string
	if err := row.Scan(
		&view.ID,
		&view.Name,
		&viewAssetID,
		&viewSource,
		&viewLevel,
		&viewSearch,
		&viewWindow,
		&view.CreatedAt,
		&view.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return logs.SavedView{}, ErrNotFound
		}
		return logs.SavedView{}, err
	}
	if viewAssetID != nil {
		view.AssetID = *viewAssetID
	}
	if viewSource != nil {
		view.Source = *viewSource
	}
	if viewLevel != nil {
		view.Level = *viewLevel
	}
	if viewSearch != nil {
		view.Search = *viewSearch
	}
	if viewWindow != nil {
		view.Window = *viewWindow
	}
	return view, nil
}

func (s *PostgresStore) DeleteView(actorID, id string) error {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		actorID = "system"
	}
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM saved_log_views WHERE owner_id = $1 AND id = $2`,
		actorID,
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
