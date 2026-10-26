package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/labtether/labtether/internal/incidents"
)

func correlationIncident(row incidents.Incident) CorrelationIncident {
	return CorrelationIncident{ID: row.ID, Status: row.Status, GroupID: row.GroupID, PrimaryAssetID: row.PrimaryAssetID, OpenedAt: row.OpenedAt.UTC()}
}

func (m *MemoryIncidentStore) ReadActiveIncidentCorrelationPage(ctx context.Context, through time.Time, cursor CorrelationCursor) ([]CorrelationIncident, CorrelationCursor, bool, error) {
	if through.IsZero() {
		return nil, cursor, false, errors.New("incident correlation cutoff required")
	}
	if err := validateCorrelationCursor(cursor); err != nil {
		return nil, cursor, false, err
	}
	ctx, cancel, err := correlationContext(ctx)
	if err != nil {
		return nil, cursor, false, err
	}
	defer cancel()
	if err = lockCorrelationRead(ctx, &m.mu); err != nil {
		return nil, cursor, false, err
	}
	defer m.mu.RUnlock()
	page := newCorrelationIDPage(cursor)
	for _, row := range m.incidents {
		if err = ctx.Err(); err != nil {
			return nil, cursor, false, err
		}
		if (row.Status == incidents.StatusOpen || row.Status == incidents.StatusInvestigating) && !row.CreatedAt.After(through) {
			if err = page.add(row.ID); err != nil {
				return nil, cursor, false, err
			}
		}
	}
	result := make([]CorrelationIncident, 0, len(page.ids))
	for _, id := range page.ids {
		result = append(result, correlationIncident(m.incidents[id]))
	}
	if err = ctx.Err(); err != nil {
		return nil, cursor, false, err
	}
	next, done := page.result()
	return result, next, done, nil
}

func (m *MemoryIncidentStore) ReadIncidentCorrelation(ctx context.Context, id string) (CorrelationIncident, bool, error) {
	if !correlationExactID(id) {
		return CorrelationIncident{}, false, errors.New("invalid incident correlation identity")
	}
	ctx, cancel, err := correlationContext(ctx)
	if err != nil {
		return CorrelationIncident{}, false, err
	}
	defer cancel()
	if err = lockCorrelationRead(ctx, &m.mu); err != nil {
		return CorrelationIncident{}, false, err
	}
	defer m.mu.RUnlock()
	if err = ctx.Err(); err != nil {
		return CorrelationIncident{}, false, err
	}
	row, found := m.incidents[id]
	if !found {
		return CorrelationIncident{}, false, nil
	}
	return correlationIncident(row), true, nil
}

func (m *MemoryActionStore) ReadActionCorrelationPage(ctx context.Context, w CorrelationWindow, scope IncidentCorrelationScope, cursor CorrelationCursor) ([]CorrelationAction, CorrelationCursor, bool, error) {
	if err := validateCorrelationQuery(w, scope); err != nil {
		return nil, cursor, false, err
	}
	if err := validateCorrelationCursor(cursor); err != nil {
		return nil, cursor, false, err
	}
	ctx, cancel, err := correlationContext(ctx)
	if err != nil {
		return nil, cursor, false, err
	}
	defer cancel()
	if err = lockCorrelationRead(ctx, &m.mu); err != nil {
		return nil, cursor, false, err
	}
	defer m.mu.RUnlock()
	page := newCorrelationIDPage(cursor)
	for _, row := range m.runs {
		if err = ctx.Err(); err != nil {
			return nil, cursor, false, err
		}
		if row.Status == "failed" && correlationInWindow(row.CreatedAt, w) && correlationContains(scope.OperationAssetIDs, row.Target) {
			if err = page.add(row.ID); err != nil {
				return nil, cursor, false, err
			}
		}
	}
	result := make([]CorrelationAction, 0, len(page.ids))
	for _, id := range page.ids {
		row := m.runs[id]
		result = append(result, CorrelationAction{ID: row.ID, Type: row.Type, Target: row.Target, CreatedAt: row.CreatedAt.UTC()})
	}
	if err = ctx.Err(); err != nil {
		return nil, cursor, false, err
	}
	next, done := page.result()
	return result, next, done, nil
}

func (m *MemoryUpdateStore) ReadUpdateCorrelationPage(ctx context.Context, w CorrelationWindow, scope IncidentCorrelationScope, cursor CorrelationCursor) ([]CorrelationUpdate, CorrelationCursor, bool, error) {
	if err := validateCorrelationQuery(w, scope); err != nil {
		return nil, cursor, false, err
	}
	if err := validateCorrelationCursor(cursor); err != nil {
		return nil, cursor, false, err
	}
	ctx, cancel, err := correlationContext(ctx)
	if err != nil {
		return nil, cursor, false, err
	}
	defer cancel()
	if err = lockCorrelationRead(ctx, &m.mu); err != nil {
		return nil, cursor, false, err
	}
	defer m.mu.RUnlock()
	page := newCorrelationIDPage(cursor)
	for _, row := range m.runs {
		if err = ctx.Err(); err != nil {
			return nil, cursor, false, err
		}
		if row.Status != "failed" || !correlationInWindow(row.CreatedAt, w) {
			continue
		}
		for _, result := range row.Results {
			if err = ctx.Err(); err != nil {
				return nil, cursor, false, err
			}
			if result.Status == "failed" && correlationContains(scope.OperationAssetIDs, result.Target) {
				if err = page.add(row.ID); err != nil {
					return nil, cursor, false, err
				}
				break
			}
		}
	}
	result := make([]CorrelationUpdate, 0, len(page.ids))
	for _, id := range page.ids {
		row := m.runs[id]
		target := ""
		for _, r := range row.Results {
			if err = ctx.Err(); err != nil {
				return nil, cursor, false, err
			}
			if r.Status == "failed" && correlationContains(scope.OperationAssetIDs, r.Target) && (target == "" || r.Target < target) {
				target = r.Target
			}
		}
		result = append(result, CorrelationUpdate{ID: row.ID, PlanID: row.PlanID, Target: target, CreatedAt: row.CreatedAt.UTC()})
	}
	if err = ctx.Err(); err != nil {
		return nil, cursor, false, err
	}
	next, done := page.result()
	return result, next, done, nil
}

func (m *MemoryAlertInstanceStore) ReadAlertCorrelationPage(ctx context.Context, w CorrelationWindow, scope IncidentCorrelationScope, cursor CorrelationCursor) ([]CorrelationAlert, CorrelationCursor, bool, error) {
	if err := validateCorrelationQuery(w, scope); err != nil {
		return nil, cursor, false, err
	}
	if err := validateCorrelationCursor(cursor); err != nil {
		return nil, cursor, false, err
	}
	ctx, cancel, err := correlationContext(ctx)
	if err != nil {
		return nil, cursor, false, err
	}
	defer cancel()
	if err = lockCorrelationRead(ctx, &m.mu); err != nil {
		return nil, cursor, false, err
	}
	defer m.mu.RUnlock()
	page := newCorrelationIDPage(cursor)
	for _, row := range m.instances {
		if err = ctx.Err(); err != nil {
			return nil, cursor, false, err
		}
		if row.FirstFiredAt == nil || !correlationInWindow(*row.FirstFiredAt, w) {
			continue
		}
		_, matched, matchErr := correlationAlertWitness(ctx, row, scope)
		if matchErr != nil {
			return nil, cursor, false, matchErr
		}
		if matched {
			if err = page.add(row.ID); err != nil {
				return nil, cursor, false, err
			}
		}
	}
	result := make([]CorrelationAlert, 0, len(page.ids))
	for _, id := range page.ids {
		row := m.instances[id]
		witness, _, matchErr := correlationAlertWitness(ctx, row, scope)
		if matchErr != nil {
			return nil, cursor, false, matchErr
		}
		result = append(result, CorrelationAlert{ID: row.ID, RuleID: row.RuleID, Fingerprint: row.Fingerprint, Severity: row.Severity, FirstFiredAt: row.FirstFiredAt.UTC(), FiringScope: witness})
	}
	if err = ctx.Err(); err != nil {
		return nil, cursor, false, err
	}
	next, done := page.result()
	return result, next, done, nil
}

func (m *MemoryAuditStore) ReadAuditCorrelationPage(ctx context.Context, w CorrelationWindow, scope IncidentCorrelationScope, cursor CorrelationCursor) ([]CorrelationAudit, CorrelationCursor, bool, error) {
	if err := validateCorrelationQuery(w, scope); err != nil {
		return nil, cursor, false, err
	}
	if err := validateCorrelationCursor(cursor); err != nil {
		return nil, cursor, false, err
	}
	ctx, cancel, err := correlationContext(ctx)
	if err != nil {
		return nil, cursor, false, err
	}
	defer cancel()
	targets := append([]string{scope.IncidentID}, scope.OperationAssetIDs...)
	events, upper, done, err := m.store.ReadCorrelationPage(ctx, w.From, w.Through, targets, cursor.AfterID, cursor.UpperID, cursor.Started)
	if err != nil {
		return nil, cursor, false, err
	}
	next := CorrelationCursor{AfterID: cursor.AfterID, UpperID: upper, Started: true}
	result := make([]CorrelationAudit, 0, len(events))
	for _, row := range events {
		result = append(result, CorrelationAudit{ID: row.ID, Type: row.Type, ActorID: row.ActorID, Target: row.Target, Timestamp: row.Timestamp.UTC()})
		next.AfterID = row.ID
	}
	return result, next, done, nil
}
