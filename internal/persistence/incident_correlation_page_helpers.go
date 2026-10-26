package persistence

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/alerts"
)

func correlationExactID(id string) bool {
	return id != "" && strings.TrimSpace(id) == id && !strings.ContainsRune(id, '\x00')
}

func correlationContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if ctx == nil {
		return nil, nil, errors.New("correlation context required")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	return bounded, cancel, nil
}

// Cancellation must also bound waiting for an existing Memory writer. No
// detached waiter remains after cancellation; callers release acquired locks.
func lockCorrelationRead(ctx context.Context, mu *sync.RWMutex) error {
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

func validateCorrelationCursor(c CorrelationCursor) error {
	if !c.Started {
		if c.AfterID != "" || c.UpperID != "" {
			return errors.New("unstarted correlation cursor has keys")
		}
		return nil
	}
	if (c.AfterID != "" && !correlationExactID(c.AfterID)) || (c.UpperID != "" && !correlationExactID(c.UpperID)) || c.AfterID > c.UpperID {
		return errors.New("invalid correlation cursor")
	}
	return nil
}

func validateCorrelationQuery(w CorrelationWindow, s IncidentCorrelationScope) error {
	if w.From.IsZero() || w.Through.IsZero() || w.From.After(w.Through) {
		return errors.New("invalid correlation window")
	}
	if !correlationExactID(s.IncidentID) || (s.GroupID != "" && !correlationExactID(s.GroupID)) {
		return errors.New("invalid incident correlation identity")
	}
	for _, ids := range [][]string{s.AssetIDs, s.OperationAssetIDs} {
		for _, id := range ids {
			if !correlationExactID(id) {
				return errors.New("invalid correlation asset identity")
			}
		}
	}
	for _, link := range s.AlertLinks {
		if link.IncidentID != s.IncidentID || (link.AlertRuleID == "" && link.AlertInstanceID == "" && link.AlertFingerprint == "") {
			return errors.New("invalid incident correlation alert link")
		}
		for _, id := range []string{link.AlertRuleID, link.AlertInstanceID, link.AlertFingerprint} {
			if id != "" && !correlationExactID(id) {
				return errors.New("invalid alert correlation discriminator")
			}
		}
	}
	return nil
}

func correlationContains(ids []string, id string) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}
func correlationInWindow(at time.Time, w CorrelationWindow) bool {
	return !at.Before(w.From) && !at.After(w.Through)
}

// Validate all evidence without cloning it. Return only one matching asset and
// group witness, so a source row with a large saved scope cannot inflate a page.
func correlationAlertWitness(ctx context.Context, row alerts.AlertInstance, scope IncidentCorrelationScope) (*alerts.FiringScope, bool, error) {
	if row.FirstFiredAt == nil || row.FirstFiredAt.IsZero() {
		return nil, false, nil
	}
	var witness *alerts.FiringScope
	if row.FiringScope != nil {
		if row.FiringScope.Version != 1 {
			return nil, false, nil
		}
		witness = &alerts.FiringScope{Version: 1}
		for kind, ids := range [][]string{row.FiringScope.AssetIDs, row.FiringScope.GroupIDs, row.FiringScope.SyntheticCheckIDs} {
			for _, id := range ids {
				if err := ctx.Err(); err != nil {
					return nil, false, err
				}
				if !correlationExactID(id) {
					return nil, false, nil
				}
				if kind == 0 && correlationContains(scope.AssetIDs, id) && (len(witness.AssetIDs) == 0 || id < witness.AssetIDs[0]) {
					witness.AssetIDs = []string{id}
				}
				if kind == 1 && scope.GroupID != "" && id == scope.GroupID {
					witness.GroupIDs = []string{id}
				}
			}
		}
	}
	for _, link := range scope.AlertLinks {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		if (link.AlertRuleID == "" || link.AlertRuleID == row.RuleID) && (link.AlertInstanceID == "" || link.AlertInstanceID == row.ID) && (link.AlertFingerprint == "" || link.AlertFingerprint == row.Fingerprint) {
			return witness, true, nil
		}
	}
	return witness, witness != nil && (len(witness.AssetIDs) > 0 || len(witness.GroupIDs) > 0), ctx.Err()
}

// Select only one bounded page while scanning Memory's map under its owner
// lock. The lock must be released before returning rows to caller work.
type correlationIDPage struct {
	cursor CorrelationCursor
	upper  string
	ids    []string
}

func newCorrelationIDPage(c CorrelationCursor) *correlationIDPage {
	return &correlationIDPage{cursor: c, upper: c.UpperID, ids: make([]string, 0, CorrelationPageSize)}
}
func (p *correlationIDPage) add(id string) error {
	if !correlationExactID(id) {
		return errors.New("invalid stored correlation ID")
	}
	if !p.cursor.Started && id > p.upper {
		p.upper = id
	}
	if id <= p.cursor.AfterID || (p.cursor.Started && id > p.cursor.UpperID) {
		return nil
	}
	i := sort.SearchStrings(p.ids, id)
	if i < len(p.ids) && p.ids[i] == id {
		return nil
	}
	if i >= CorrelationPageSize {
		return nil
	}
	if len(p.ids) < CorrelationPageSize {
		p.ids = append(p.ids, "")
	}
	copy(p.ids[i+1:], p.ids[i:])
	p.ids[i] = id
	return nil
}
func (p *correlationIDPage) result() (CorrelationCursor, bool) {
	next := CorrelationCursor{Started: true, UpperID: p.upper, AfterID: p.cursor.AfterID}
	if len(p.ids) > 0 {
		next.AfterID = p.ids[len(p.ids)-1]
	}
	return next, len(p.ids) < CorrelationPageSize
}
