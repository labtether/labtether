package persistence

import (
	"context"
	"errors"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/incidents"
	"strings"
	"time"
)

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

	out := make([]incidents.AlertLink, 0)
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
