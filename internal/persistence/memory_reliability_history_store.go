package persistence

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/telemetry"
)

// InsertReliabilityRecord implements ReliabilityHistoryStore for the in-memory
// group store so tests and non-Postgres configurations retain production parity.
func (m *MemoryGroupStore) InsertReliabilityRecord(groupID string, score int, grade string, factors map[string]any, windowHours int) error {
	groupID = strings.TrimSpace(groupID)
	record := ReliabilityRecord{
		ID:          idgen.New("rh"),
		GroupID:     groupID,
		Score:       score,
		Grade:       strings.TrimSpace(grade),
		Factors:     cloneAnyMap(factors),
		WindowHours: windowHours,
		ComputedAt:  time.Now().UTC(),
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.groups[groupID]; !exists {
		return ErrNotFound
	}
	m.reliabilityHistory[groupID] = append(m.reliabilityHistory[groupID], record)
	return nil
}

func cloneReliabilityRecord(record ReliabilityRecord) ReliabilityRecord {
	record.Factors = cloneAnyMap(record.Factors)
	return record
}

// ListReliabilityHistory implements ReliabilityHistoryStore with the same day
// clamp and newest-first ordering as PostgreSQL.
func (m *MemoryGroupStore) ListReliabilityHistory(groupID string, days int) ([]ReliabilityRecord, error) {
	if days <= 0 {
		days = 7
	}
	if days > 365 {
		days = 365
	}
	oldest := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	m.mu.RLock()
	records := m.reliabilityHistory[strings.TrimSpace(groupID)]
	out := make([]ReliabilityRecord, 0, len(records))
	for _, record := range records {
		if record.ComputedAt.After(oldest) {
			out = append(out, cloneReliabilityRecord(record))
		}
	}
	m.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].ComputedAt.Equal(out[j].ComputedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].ComputedAt.After(out[j].ComputedAt)
	})
	return out, nil
}

// PruneReliabilityHistory implements ReliabilityHistoryStore.
func (m *MemoryGroupStore) PruneReliabilityHistory(olderThanDays int) (int64, error) {
	if olderThanDays <= 0 {
		olderThanDays = 90
	}
	oldest := time.Now().UTC().Add(-time.Duration(olderThanDays) * 24 * time.Hour)
	var removed int64
	m.mu.Lock()
	defer m.mu.Unlock()
	for groupID, records := range m.reliabilityHistory {
		kept := records[:0]
		for _, record := range records {
			if record.ComputedAt.Before(oldest) {
				removed++
				continue
			}
			kept = append(kept, record)
		}
		if len(kept) == 0 {
			delete(m.reliabilityHistory, groupID)
		} else {
			m.reliabilityHistory[groupID] = kept
		}
	}
	return removed, nil
}

// LatestReliabilityMetricSnapshots returns at most maxGroups latest-per-group
// scores in stable group-ID order without an N+1 history lookup.
func (m *MemoryGroupStore) LatestReliabilityMetricSnapshots(ctx context.Context, at time.Time, maxGroups int) ([]ReliabilityMetricSnapshot, error) {
	if ctx == nil {
		return nil, fmt.Errorf("reliability metric snapshot context is required")
	}
	if maxGroups <= 0 || maxGroups > telemetry.MaxSiteReliabilityMetricSeries {
		return nil, ErrReliabilityMetricSnapshotLimitExceeded
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}
	oldest := at.Add(-24 * time.Hour)

	m.mu.RLock()
	defer m.mu.RUnlock()
	groupIDs := make([]string, 0, len(m.groups))
	for groupID := range m.groups {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Strings(groupIDs)
	if len(groupIDs) > maxGroups {
		groupIDs = groupIDs[:maxGroups]
	}
	out := make([]ReliabilityMetricSnapshot, 0, len(groupIDs))
	for i, groupID := range groupIDs {
		if i%128 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		records := m.reliabilityHistory[groupID]
		var latest ReliabilityRecord
		found := false
		for j := 0; j < len(records); j++ {
			if j%256 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			candidate := records[j]
			if candidate.ComputedAt.After(at) || !candidate.ComputedAt.After(oldest) {
				continue
			}
			if !found || candidate.ComputedAt.After(latest.ComputedAt) ||
				(candidate.ComputedAt.Equal(latest.ComputedAt) && candidate.ID > latest.ID) {
				latest = candidate
				found = true
			}
		}
		if !found {
			continue
		}
		group := m.groups[groupID]
		out = append(out, ReliabilityMetricSnapshot{GroupID: groupID, GroupName: group.Name, Score: latest.Score})
	}
	return out, nil
}
