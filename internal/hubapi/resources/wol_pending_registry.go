package resources

import (
	"strings"
	"sync"
	"time"
)

type pendingWoLRelay struct {
	ActorID     string
	TargetID    string
	RelayID     string
	ExpectedMAC string
	RequestRef  string
	ExpiresAt   time.Time
}

type wolPendingConsumeStatus uint8

const (
	wolPendingConsumed wolPendingConsumeStatus = iota
	wolPendingUnknown
	wolPendingExpired
	wolPendingRelayMismatch
	wolPendingMACMismatch
)

// WoLPendingRegistry is a bounded, process-local registry for agent-assisted
// Wake-on-LAN requests. It prevents unsolicited, replayed, cross-relay, and
// wrong-target results from being promoted into successful audit outcomes.
// The zero value is ready for use.
type WoLPendingRegistry struct {
	mu           sync.Mutex
	entries      map[string]pendingWoLRelay
	auditWindows map[string]time.Time
}

func (r *WoLPendingRegistry) store(requestID string, pending pendingWoLRelay) bool {
	if r == nil || strings.TrimSpace(requestID) == "" {
		return false
	}
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneExpiredLocked(now)
	if r.entries == nil {
		r.entries = make(map[string]pendingWoLRelay, 16)
	}
	if len(r.entries) >= maxPendingWoLRelay {
		return false
	}
	if _, exists := r.entries[requestID]; exists {
		return false
	}
	r.entries[requestID] = pending
	return true
}

func (r *WoLPendingRegistry) delete(requestID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	delete(r.entries, requestID)
	r.mu.Unlock()
}

func (r *WoLPendingRegistry) refreshExpiry(requestID string, expiresAt time.Time) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	pending, ok := r.entries[requestID]
	if !ok {
		return false
	}
	pending.ExpiresAt = expiresAt
	r.entries[requestID] = pending
	return true
}

func (r *WoLPendingRegistry) consume(requestID, relayID, canonicalMAC string) (pendingWoLRelay, wolPendingConsumeStatus) {
	if r == nil {
		return pendingWoLRelay{}, wolPendingUnknown
	}
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	pending, ok := r.entries[requestID]
	if !ok {
		return pendingWoLRelay{}, wolPendingUnknown
	}
	if !pending.ExpiresAt.After(now) {
		delete(r.entries, requestID)
		return pending, wolPendingExpired
	}
	if !strings.EqualFold(strings.TrimSpace(pending.RelayID), strings.TrimSpace(relayID)) {
		return pending, wolPendingRelayMismatch
	}
	if !strings.EqualFold(pending.ExpectedMAC, canonicalMAC) {
		return pending, wolPendingMACMismatch
	}
	delete(r.entries, requestID)
	return pending, wolPendingConsumed
}

func (r *WoLPendingRegistry) expire(requestID string) (pendingWoLRelay, bool) {
	if r == nil {
		return pendingWoLRelay{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	pending, ok := r.entries[requestID]
	if ok {
		delete(r.entries, requestID)
	}
	return pending, ok
}

func (r *WoLPendingRegistry) pruneExpiredLocked(now time.Time) {
	for requestID, pending := range r.entries {
		if !pending.ExpiresAt.After(now) {
			delete(r.entries, requestID)
		}
	}
}

func (r *WoLPendingRegistry) allowAudit(key string, now time.Time) bool {
	if r == nil || strings.TrimSpace(key) == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.auditWindows == nil {
		r.auditWindows = make(map[string]time.Time, 16)
	}
	for candidate, expiresAt := range r.auditWindows {
		if !expiresAt.After(now) {
			delete(r.auditWindows, candidate)
		}
	}
	if expiresAt, exists := r.auditWindows[key]; exists && expiresAt.After(now) {
		return false
	}
	if len(r.auditWindows) >= maxWoLAuditWindows {
		var oldestKey string
		var oldestExpiry time.Time
		for candidate, expiresAt := range r.auditWindows {
			if oldestExpiry.IsZero() || expiresAt.Before(oldestExpiry) {
				oldestKey = candidate
				oldestExpiry = expiresAt
			}
		}
		delete(r.auditWindows, oldestKey)
	}
	r.auditWindows[key] = now.Add(wolAuditThrottleTTL)
	return true
}
