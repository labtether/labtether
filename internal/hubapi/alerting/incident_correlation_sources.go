package alerting

import (
	"context"
	"fmt"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/updates"
)

type correlationCandidate struct {
	id       string
	data     correlationPage
	rejected bool
}

// Each adapter retains only one small page and reuses the actual event matcher.
// A configured store without the context-aware page contract is an error.
func (d *Deps) readCorrelationPage(ctx context.Context, source int, window persistence.CorrelationWindow, scope persistence.IncidentCorrelationScope, cursor persistence.CorrelationCursor) ([]correlationCandidate, persistence.CorrelationCursor, bool, error) {
	empty := persistence.CorrelationCursor{Started: true}
	switch source {
	case 0:
		if d.ActionStore == nil {
			return nil, empty, true, nil
		}
		owner, ok := d.ActionStore.(persistence.ActionCorrelationPager)
		if !ok {
			return nil, cursor, false, fmt.Errorf("action correlation owner unavailable")
		}
		rows, next, done, err := owner.ReadActionCorrelationPage(ctx, window, scope, cursor)
		if err != nil {
			return nil, cursor, false, err
		}
		out := make([]correlationCandidate, 0, len(rows))
		for _, row := range rows {
			out = append(out, correlationCandidate{id: row.ID, data: correlationPage{actionRuns: []actions.Run{
				{ID: row.ID, Type: row.Type, Target: row.Target, Status: actions.StatusFailed, CreatedAt: row.CreatedAt},
			}}})
		}
		return out, next, done, nil
	case 1:
		if d.UpdateStore == nil {
			return nil, empty, true, nil
		}
		owner, ok := d.UpdateStore.(persistence.UpdateCorrelationPager)
		if !ok {
			return nil, cursor, false, fmt.Errorf("update correlation owner unavailable")
		}
		rows, next, done, err := owner.ReadUpdateCorrelationPage(ctx, window, scope, cursor)
		if err != nil {
			return nil, cursor, false, err
		}
		out := make([]correlationCandidate, 0, len(rows))
		for _, row := range rows {
			out = append(out, correlationCandidate{id: row.ID, data: correlationPage{updateRuns: []updates.Run{
				{ID: row.ID, PlanID: row.PlanID, Status: updates.StatusFailed, CreatedAt: row.CreatedAt,
					Results: []updates.RunResultEntry{{Target: row.Target, Status: updates.StatusFailed}}},
			}}})
		}
		return out, next, done, nil
	case 2:
		if d.AlertInstanceStore == nil {
			return nil, empty, true, nil
		}
		owner, ok := d.AlertInstanceStore.(persistence.AlertCorrelationPager)
		if !ok {
			return nil, cursor, false, fmt.Errorf("alert correlation owner unavailable")
		}
		rows, next, done, err := owner.ReadAlertCorrelationPage(ctx, window, scope, cursor)
		if err != nil {
			return nil, cursor, false, err
		}
		out := make([]correlationCandidate, 0, len(rows))
		for _, row := range rows {
			out = append(out, correlationCandidate{id: row.ID, rejected: row.Rejected, data: correlationPage{alertInstances: []alerts.AlertInstance{
				{ID: row.ID, RuleID: row.RuleID, Fingerprint: row.Fingerprint, Severity: row.Severity,
					FirstFiredAt: &row.FirstFiredAt, FiringScope: row.FiringScope},
			}}})
		}
		return out, next, done, nil
	case 3:
		if d.AuditStore == nil {
			return nil, empty, true, nil
		}
		owner, ok := d.AuditStore.(persistence.AuditCorrelationPager)
		if !ok {
			return nil, cursor, false, fmt.Errorf("audit correlation owner unavailable")
		}
		rows, next, done, err := owner.ReadAuditCorrelationPage(ctx, window, scope, cursor)
		if err != nil {
			return nil, cursor, false, err
		}
		out := make([]correlationCandidate, 0, len(rows))
		for _, row := range rows {
			out = append(out, correlationCandidate{id: row.ID, data: correlationPage{auditEvents: []audit.Event{
				{ID: row.ID, Type: row.Type, ActorID: row.ActorID, Target: row.Target, Timestamp: row.Timestamp},
			}}})
		}
		return out, next, done, nil
	default:
		return nil, cursor, false, fmt.Errorf("unknown incident correlation source")
	}
}
