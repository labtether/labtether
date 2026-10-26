package persistence

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/incidents"
)

var (
	_ IncidentAlertScopeReader = (*MemoryIncidentStore)(nil)
	_ GroupAssetIDScopeReader  = (*MemoryAssetStore)(nil)
)

// Avoid an unbounded wait on an in-flight writer. No waiter goroutine survives
// cancellation; the acquired read lock is always released by the caller.
func lockIncidentScopeRead(ctx context.Context, mu *sync.RWMutex) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if mu.TryRLock() {
			return nil
		}
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (m *MemoryIncidentStore) ReadIncidentAlertLinksContext(ctx context.Context, incidentID string, visit func(incidents.AlertLink) error) error {
	ctx, cancel := context.WithTimeout(ctx, incidentScopeReadTimeout)
	defer cancel()
	if !validIncidentScopeID(incidentID) || visit == nil {
		return fmt.Errorf("invalid incident alert scope read")
	}
	if err := lockIncidentScopeRead(ctx, &m.mu); err != nil {
		return err
	}
	defer m.mu.RUnlock()
	if _, found := m.incidents[incidentID]; !found {
		return incidents.ErrIncidentNotFound
	}
	rows := make([]incidents.AlertLink, 0, len(m.links[incidentID]))
	for _, row := range m.links[incidentID] {
		if err := ctx.Err(); err != nil {
			return err
		}
		rows = append(rows, cloneIncidentAlertLink(row))
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	after := ""
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		if row.ID <= after {
			return fmt.Errorf("invalid incident alert scope order")
		}
		if err := validateIncidentAlertScope(row, incidentID); err != nil {
			return err
		}
		if err := visit(row); err != nil {
			return err
		}
		after = row.ID
	}
	return ctx.Err()
}

func (m *MemoryAssetStore) ReadGroupAssetIDsContext(ctx context.Context, groupID string, visit func(string) error) error {
	ctx, cancel := context.WithTimeout(ctx, incidentScopeReadTimeout)
	defer cancel()
	if !validIncidentScopeID(groupID) || visit == nil {
		return fmt.Errorf("invalid incident group scope read")
	}
	if err := lockIncidentScopeRead(ctx, &m.mu); err != nil {
		return err
	}
	defer m.mu.RUnlock()
	ids := make([]string, 0)
	for key, row := range m.assets {
		if err := ctx.Err(); err != nil {
			return err
		}
		if row.GroupID == groupID {
			if row.ID != key || !validIncidentScopeID(row.ID) {
				return fmt.Errorf("invalid incident group asset identity")
			}
			ids = append(ids, row.ID)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := visit(id); err != nil {
			return err
		}
	}
	return ctx.Err()
}
