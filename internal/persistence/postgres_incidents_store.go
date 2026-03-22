package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/incidents"
)

func (s *PostgresStore) CreateIncident(req incidents.CreateIncidentRequest) (incidents.Incident, error) {
	now := time.Now().UTC()
	source := incidents.NormalizeSource(req.Source)
	if source == "" {
		source = incidents.SourceManual
	}
	severity := incidents.NormalizeSeverity(req.Severity)
	if severity == "" {
		severity = incidents.SeverityMedium
	}
	createdBy := strings.TrimSpace(req.CreatedBy)
	if createdBy == "" {
		createdBy = "owner"
	}

	metadataPayload, err := marshalStringMap(req.Metadata)
	if err != nil {
		return incidents.Incident{}, err
	}

	return scanIncident(s.pool.QueryRow(context.Background(),
		`INSERT INTO incidents (
			id,
			title,
			summary,
			status,
			severity,
			source,
			group_id,
			primary_asset_id,
			assignee,
			created_by,
			opened_at,
			metadata,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13, $13)
		RETURNING
			id,
			title,
			summary,
			status,
			severity,
			source,
			group_id,
			primary_asset_id,
			assignee,
			created_by,
			opened_at,
			mitigated_at,
			resolved_at,
			closed_at,
			metadata,
			root_cause,
			action_items,
			lessons_learned,
			created_at,
			updated_at`,
		idgen.New("inc"),
		strings.TrimSpace(req.Title),
		strings.TrimSpace(req.Summary),
		incidents.StatusOpen,
		severity,
		source,
		nullIfBlank(req.GroupID),
		nullIfBlank(req.PrimaryAssetID),
		nullIfBlank(req.Assignee),
		createdBy,
		now,
		metadataPayload,
		now,
	))
}

func (s *PostgresStore) GetIncident(id string) (incidents.Incident, bool, error) {
	incident, err := scanIncident(s.pool.QueryRow(context.Background(),
		`SELECT
			id,
			title,
			summary,
			status,
			severity,
			source,
			group_id,
			primary_asset_id,
			assignee,
			created_by,
			opened_at,
			mitigated_at,
			resolved_at,
			closed_at,
			metadata,
			root_cause,
			action_items,
			lessons_learned,
			created_at,
			updated_at
		 FROM incidents
		 WHERE id = $1`,
		strings.TrimSpace(id),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return incidents.Incident{}, false, nil
		}
		return incidents.Incident{}, false, err
	}
	return incident, true, nil
}

func (s *PostgresStore) ListIncidents(filter IncidentFilter) ([]incidents.Incident, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	where := make([]string, 0, 5)
	args := make([]any, 0, 6)
	next := 1

	if status := incidents.NormalizeStatus(filter.Status); status != "" {
		where = append(where, fmt.Sprintf("status = $%d", next))
		args = append(args, status)
		next++
	}
	if severity := incidents.NormalizeSeverity(filter.Severity); severity != "" {
		where = append(where, fmt.Sprintf("severity = $%d", next))
		args = append(args, severity)
		next++
	}
	if source := incidents.NormalizeSource(filter.Source); source != "" {
		where = append(where, fmt.Sprintf("source = $%d", next))
		args = append(args, source)
		next++
	}
	if groupID := strings.TrimSpace(filter.GroupID); groupID != "" {
		where = append(where, fmt.Sprintf("group_id = $%d", next))
		args = append(args, groupID)
		next++
	}
	if assignee := strings.TrimSpace(filter.Assignee); assignee != "" {
		where = append(where, fmt.Sprintf("assignee = $%d", next))
		args = append(args, assignee)
		next++
	}

	sql := `SELECT
		id,
		title,
		summary,
		status,
		severity,
		source,
		group_id,
		primary_asset_id,
		assignee,
		created_by,
		opened_at,
		mitigated_at,
		resolved_at,
		closed_at,
		metadata,
		root_cause,
		action_items,
		lessons_learned,
		created_at,
		updated_at
	FROM incidents`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d", next)
	args = append(args, limit)
	next++
	if filter.Offset > 0 {
		args = append(args, filter.Offset)
		sql += fmt.Sprintf(" OFFSET $%d", next)
	}

	rows, err := s.pool.Query(context.Background(), sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]incidents.Incident, 0, limit)
	for rows.Next() {
		incident, scanErr := scanIncident(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, incident)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) UpdateIncident(id string, req incidents.UpdateIncidentRequest) (incidents.Incident, error) {
	incident, ok, err := s.GetIncident(strings.TrimSpace(id))
	if err != nil {
		return incidents.Incident{}, err
	}
	if !ok {
		return incidents.Incident{}, incidents.ErrIncidentNotFound
	}

	if req.Title != nil {
		incident.Title = strings.TrimSpace(*req.Title)
	}
	if req.Summary != nil {
		incident.Summary = strings.TrimSpace(*req.Summary)
	}
	if req.Severity != nil {
		nextSeverity := incidents.NormalizeSeverity(*req.Severity)
		if nextSeverity == "" {
			return incidents.Incident{}, errors.New("invalid incident severity")
		}
		incident.Severity = nextSeverity
	}
	if req.Assignee != nil {
		incident.Assignee = strings.TrimSpace(*req.Assignee)
	}
	if req.GroupID != nil {
		incident.GroupID = strings.TrimSpace(*req.GroupID)
	}
	if req.PrimaryAssetID != nil {
		incident.PrimaryAssetID = strings.TrimSpace(*req.PrimaryAssetID)
	}
	if req.Metadata != nil {
		incident.Metadata = cloneMetadata(*req.Metadata)
	}
	if req.RootCause != nil {
		incident.RootCause = strings.TrimSpace(*req.RootCause)
	}
	if req.ActionItems != nil {
		incident.ActionItems = *req.ActionItems
	}
	if req.LessonsLearned != nil {
		incident.LessonsLearned = strings.TrimSpace(*req.LessonsLearned)
	}
	if req.Status != nil {
		nextStatus := incidents.NormalizeStatus(*req.Status)
		if nextStatus == "" || !incidents.CanTransitionStatus(incident.Status, nextStatus) {
			return incidents.Incident{}, incidents.ErrInvalidStatusTransition
		}
		if incident.Status != nextStatus {
			incident.Status = nextStatus
			now := time.Now().UTC()
			switch nextStatus {
			case incidents.StatusMitigated:
				if incident.MitigatedAt == nil {
					incident.MitigatedAt = &now
				}
			case incidents.StatusResolved:
				if incident.ResolvedAt == nil {
					incident.ResolvedAt = &now
				}
			case incidents.StatusClosed:
				if incident.ClosedAt == nil {
					incident.ClosedAt = &now
				}
			}
		}
	}

	metadataPayload, err := marshalStringMap(incident.Metadata)
	if err != nil {
		return incidents.Incident{}, err
	}

	actionItemsPayload, err := json.Marshal(incident.ActionItems)
	if err != nil {
		return incidents.Incident{}, err
	}
	if incident.ActionItems == nil {
		actionItemsPayload = []byte("[]")
	}

	updatedAt := time.Now().UTC()
	updated, err := scanIncident(s.pool.QueryRow(context.Background(),
		`UPDATE incidents
		 SET title = $2,
		     summary = $3,
		     status = $4,
		     severity = $5,
		     group_id = $6,
		     primary_asset_id = $7,
		     assignee = $8,
		     metadata = $9::jsonb,
		     mitigated_at = $10,
		     resolved_at = $11,
		     closed_at = $12,
		     root_cause = $13,
		     action_items = $14::jsonb,
		     lessons_learned = $15,
		     updated_at = $16
		 WHERE id = $1
		 RETURNING
			id,
			title,
			summary,
			status,
			severity,
			source,
			group_id,
			primary_asset_id,
			assignee,
			created_by,
			opened_at,
			mitigated_at,
			resolved_at,
			closed_at,
			metadata,
			root_cause,
			action_items,
			lessons_learned,
			created_at,
			updated_at`,
		incident.ID,
		incident.Title,
		incident.Summary,
		incident.Status,
		incident.Severity,
		nullIfBlank(incident.GroupID),
		nullIfBlank(incident.PrimaryAssetID),
		nullIfBlank(incident.Assignee),
		metadataPayload,
		nullTime(incident.MitigatedAt),
		nullTime(incident.ResolvedAt),
		nullTime(incident.ClosedAt),
		incident.RootCause,
		actionItemsPayload,
		incident.LessonsLearned,
		updatedAt,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return incidents.Incident{}, incidents.ErrIncidentNotFound
		}
		return incidents.Incident{}, err
	}
	return updated, nil
}

func (s *PostgresStore) DeleteIncident(id string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM incidents WHERE id = $1`,
		strings.TrimSpace(id),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return incidents.ErrIncidentNotFound
	}
	return nil
}

func (s *PostgresStore) LinkIncidentAlert(incidentID string, req incidents.LinkAlertRequest) (incidents.AlertLink, error) {
	incidentID = strings.TrimSpace(incidentID)
	var incidentExists bool
	if err := s.pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM incidents WHERE id = $1)`,
		incidentID,
	).Scan(&incidentExists); err != nil {
		return incidents.AlertLink{}, err
	}
	if !incidentExists {
		return incidents.AlertLink{}, incidents.ErrIncidentNotFound
	}

	linkType := incidents.NormalizeLinkType(req.LinkType)
	if linkType == "" {
		return incidents.AlertLink{}, errors.New("invalid incident alert link_type")
	}
	alertRuleID := strings.TrimSpace(req.AlertRuleID)
	alertInstanceID := strings.TrimSpace(req.AlertInstanceID)
	alertFingerprint := strings.TrimSpace(req.AlertFingerprint)
	if alertRuleID == "" && alertInstanceID == "" && alertFingerprint == "" {
		return incidents.AlertLink{}, incidents.ErrAlertReferenceRequired
	}

	if alertRuleID != "" {
		var conflict bool
		if err := s.pool.QueryRow(context.Background(),
			`SELECT EXISTS (
				SELECT 1
				FROM incident_alert_links
				WHERE incident_id = $1
				  AND alert_rule_id = $2
			)`,
			incidentID,
			alertRuleID,
		).Scan(&conflict); err != nil {
			return incidents.AlertLink{}, err
		}
		if conflict {
			return incidents.AlertLink{}, incidents.ErrIncidentAlertLinkConflict
		}
	}
	if alertInstanceID != "" {
		var conflict bool
		if err := s.pool.QueryRow(context.Background(),
			`SELECT EXISTS (
				SELECT 1
				FROM incident_alert_links
				WHERE incident_id = $1
				  AND alert_instance_id = $2
			)`,
			incidentID,
			alertInstanceID,
		).Scan(&conflict); err != nil {
			return incidents.AlertLink{}, err
		}
		if conflict {
			return incidents.AlertLink{}, incidents.ErrIncidentAlertLinkConflict
		}
	}

	createdBy := strings.TrimSpace(req.CreatedBy)
	if createdBy == "" {
		createdBy = "owner"
	}
	now := time.Now().UTC()

	return scanIncidentAlertLink(s.pool.QueryRow(context.Background(),
		`INSERT INTO incident_alert_links (
			id,
			incident_id,
			alert_rule_id,
			alert_instance_id,
			alert_fingerprint,
			link_type,
			created_by,
			created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING
			id,
			incident_id,
			alert_rule_id,
			alert_instance_id,
			alert_fingerprint,
			link_type,
			created_by,
			created_at`,
		idgen.New("inclink"),
		incidentID,
		nullIfBlank(alertRuleID),
		nullIfBlank(alertInstanceID),
		nullIfBlank(alertFingerprint),
		linkType,
		createdBy,
		now,
	))
}

func (s *PostgresStore) ListIncidentAlertLinks(incidentID string, limit int) ([]incidents.AlertLink, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	incidentID = strings.TrimSpace(incidentID)

	var incidentExists bool
	if err := s.pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM incidents WHERE id = $1)`,
		incidentID,
	).Scan(&incidentExists); err != nil {
		return nil, err
	}
	if !incidentExists {
		return nil, incidents.ErrIncidentNotFound
	}

	rows, err := s.pool.Query(context.Background(),
		`SELECT
			id,
			incident_id,
			alert_rule_id,
			alert_instance_id,
			alert_fingerprint,
			link_type,
			created_by,
			created_at
		 FROM incident_alert_links
		 WHERE incident_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2`,
		incidentID,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]incidents.AlertLink, 0, limit)
	for rows.Next() {
		link, scanErr := scanIncidentAlertLink(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, link)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) UnlinkIncidentAlert(incidentID, linkID string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM incident_alert_links WHERE id = $1 AND incident_id = $2`,
		strings.TrimSpace(linkID),
		strings.TrimSpace(incidentID),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) HasAutoIncidentForAlertInstance(alertInstanceID string) (bool, error) {
	alertInstanceID = strings.TrimSpace(alertInstanceID)
	if alertInstanceID == "" {
		return false, nil
	}

	var exists bool
	err := s.pool.QueryRow(
		context.Background(),
		`SELECT EXISTS (
			SELECT 1
			FROM incident_alert_links links
			INNER JOIN incidents incidents ON incidents.id = links.incident_id
			WHERE links.alert_instance_id = $1
			  AND incidents.source = $2
		)`,
		alertInstanceID,
		incidents.SourceAlertAuto,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}
