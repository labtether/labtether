package persistence

import (
	"container/heap"
	"context"
	"sort"
	"time"

	"github.com/labtether/labtether/internal/alerts"
)

// Only the smallest page of IDs is retained while scanning under the read lock;
// no full cloned rule inventory or external callback is held by this owner.
func (m *MemoryAlertStore) PageActiveAlertRules(ctx context.Context, cursor AlertRuleEvaluationCursor, limit int) (AlertRuleEvaluationPage, error) {
	if err := validateAlertRuleEvaluationPage(cursor, limit); err != nil {
		return AlertRuleEvaluationPage{}, err
	}
	// A writer must not make this context-aware read wait without a bound.
	// No goroutine is left waiting on the lock after cancellation.
	for {
		if err := ctx.Err(); err != nil {
			return AlertRuleEvaluationPage{}, err
		}
		if m.mu.TryRLock() {
			break
		}
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return AlertRuleEvaluationPage{}, ctx.Err()
		case <-timer.C:
		}
	}
	defer m.mu.RUnlock()
	out := AlertRuleEvaluationPage{ThroughID: cursor.ThroughID, Rules: []alerts.Rule{}}
	ids := &alertRulePageIDs{}
	visited := 0
	for id, rule := range m.rules {
		visited++
		if visited%128 == 0 {
			if err := ctx.Err(); err != nil {
				return AlertRuleEvaluationPage{}, err
			}
		}
		if rule.Status != alerts.RuleStatusActive {
			continue
		}
		if !cursor.Started && id > out.ThroughID {
			out.ThroughID = id
		}
		if id <= cursor.AfterID || (cursor.Started && id > cursor.ThroughID) {
			continue
		}
		if ids.Len() < limit+1 {
			heap.Push(ids, id)
		} else if id < (*ids)[0] {
			(*ids)[0] = id
			heap.Fix(ids, 0)
		}
	}
	out.Done = ids.Len() <= limit
	sort.Strings(*ids)
	if ids.Len() > limit {
		*ids = (*ids)[:limit]
	}
	for _, id := range *ids {
		out.Rules = append(out.Rules, cloneAlertRule(m.rules[id]))
	}
	return out, ctx.Err()
}

type alertRulePageIDs []string

func (h alertRulePageIDs) Len() int           { return len(h) }
func (h alertRulePageIDs) Less(i, j int) bool { return h[i] > h[j] }
func (h alertRulePageIDs) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *alertRulePageIDs) Push(v any)        { *h = append(*h, v.(string)) }
func (h *alertRulePageIDs) Pop() any {
	last := (*h)[len(*h)-1]
	*h = (*h)[:len(*h)-1]
	return last
}
