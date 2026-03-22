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

func (s *PostgresStore) CreateAlertRoute(req notifications.CreateRouteRequest) (notifications.Route, error) {
	now := time.Now().UTC()
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	groupWait := req.GroupWaitSeconds
	if groupWait <= 0 {
		groupWait = 30
	}
	groupInterval := req.GroupIntervalSeconds
	if groupInterval <= 0 {
		groupInterval = 300
	}
	repeatInterval := req.RepeatIntervalSeconds
	if repeatInterval <= 0 {
		repeatInterval = 3600
	}

	matchersPayload, err := marshalAnyMap(req.Matchers)
	if err != nil {
		return notifications.Route{}, err
	}
	channelIDsPayload, err := marshalStringSlice(req.ChannelIDs)
	if err != nil {
		return notifications.Route{}, err
	}
	groupByPayload, err := marshalStringSlice(req.GroupBy)
	if err != nil {
		return notifications.Route{}, err
	}

	return scanAlertRoute(s.pool.QueryRow(context.Background(),
		`INSERT INTO alert_routes (
			id, name, matchers, channel_ids, severity_filter, group_filter,
			group_by, group_wait_seconds, group_interval_seconds, repeat_interval_seconds,
			enabled, created_at, updated_at
		)
		VALUES ($1, $2, $3::jsonb, $4::jsonb, $5, $6, $7::jsonb, $8, $9, $10, $11, $12, $12)
		RETURNING id, name, matchers, channel_ids, severity_filter, group_filter,
			group_by, group_wait_seconds, group_interval_seconds, repeat_interval_seconds,
			enabled, created_at, updated_at`,
		idgen.New("aroute"),
		strings.TrimSpace(req.Name),
		matchersPayload,
		channelIDsPayload,
		nullIfBlank(req.SeverityFilter),
		nullIfBlank(req.GroupFilter),
		groupByPayload,
		groupWait,
		groupInterval,
		repeatInterval,
		enabled,
		now,
	))
}

func (s *PostgresStore) GetAlertRoute(id string) (notifications.Route, bool, error) {
	route, err := scanAlertRoute(s.pool.QueryRow(context.Background(),
		`SELECT id, name, matchers, channel_ids, severity_filter, group_filter,
			group_by, group_wait_seconds, group_interval_seconds, repeat_interval_seconds,
			enabled, created_at, updated_at
		 FROM alert_routes WHERE id = $1`,
		strings.TrimSpace(id),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return notifications.Route{}, false, nil
		}
		return notifications.Route{}, false, err
	}
	return route, true, nil
}

func (s *PostgresStore) ListAlertRoutes(limit int) ([]notifications.Route, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.pool.Query(context.Background(),
		`SELECT id, name, matchers, channel_ids, severity_filter, group_filter,
			group_by, group_wait_seconds, group_interval_seconds, repeat_interval_seconds,
			enabled, created_at, updated_at
		 FROM alert_routes ORDER BY updated_at DESC LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]notifications.Route, 0, limit)
	for rows.Next() {
		route, scanErr := scanAlertRoute(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, route)
	}
	return out, rows.Err()
}

func (s *PostgresStore) UpdateAlertRoute(id string, req notifications.UpdateRouteRequest) (notifications.Route, error) {
	existing, ok, err := s.GetAlertRoute(id)
	if err != nil {
		return notifications.Route{}, err
	}
	if !ok {
		return notifications.Route{}, notifications.ErrRouteNotFound
	}

	if req.Name != nil {
		existing.Name = strings.TrimSpace(*req.Name)
	}
	if req.Matchers != nil {
		existing.Matchers = *req.Matchers
	}
	if req.ChannelIDs != nil {
		existing.ChannelIDs = *req.ChannelIDs
	}
	if req.SeverityFilter != nil {
		existing.SeverityFilter = strings.TrimSpace(*req.SeverityFilter)
	}
	if req.GroupFilter != nil {
		existing.GroupFilter = strings.TrimSpace(*req.GroupFilter)
	}
	if req.GroupBy != nil {
		existing.GroupBy = *req.GroupBy
	}
	if req.GroupWaitSeconds != nil {
		existing.GroupWaitSeconds = *req.GroupWaitSeconds
	}
	if req.GroupIntervalSeconds != nil {
		existing.GroupIntervalSeconds = *req.GroupIntervalSeconds
	}
	if req.RepeatIntervalSeconds != nil {
		existing.RepeatIntervalSeconds = *req.RepeatIntervalSeconds
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}

	matchersPayload, err := marshalAnyMap(existing.Matchers)
	if err != nil {
		return notifications.Route{}, err
	}
	channelIDsPayload, err := marshalStringSlice(existing.ChannelIDs)
	if err != nil {
		return notifications.Route{}, err
	}
	groupByPayload, err := marshalStringSlice(existing.GroupBy)
	if err != nil {
		return notifications.Route{}, err
	}

	now := time.Now().UTC()
	return scanAlertRoute(s.pool.QueryRow(context.Background(),
		`UPDATE alert_routes
		 SET name = $2, matchers = $3::jsonb, channel_ids = $4::jsonb,
		     severity_filter = $5, group_filter = $6, group_by = $7::jsonb,
		     group_wait_seconds = $8, group_interval_seconds = $9,
		     repeat_interval_seconds = $10, enabled = $11, updated_at = $12
		 WHERE id = $1
		 RETURNING id, name, matchers, channel_ids, severity_filter, group_filter,
			group_by, group_wait_seconds, group_interval_seconds, repeat_interval_seconds,
			enabled, created_at, updated_at`,
		existing.ID,
		existing.Name,
		matchersPayload,
		channelIDsPayload,
		nullIfBlank(existing.SeverityFilter),
		nullIfBlank(existing.GroupFilter),
		groupByPayload,
		existing.GroupWaitSeconds,
		existing.GroupIntervalSeconds,
		existing.RepeatIntervalSeconds,
		existing.Enabled,
		now,
	))
}

func (s *PostgresStore) DeleteAlertRoute(id string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM alert_routes WHERE id = $1`,
		strings.TrimSpace(id),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return notifications.ErrRouteNotFound
	}
	return nil
}
