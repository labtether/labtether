package persistence

import (
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/idgen"
)

// MemoryLinkSuggestionStore is an in-memory LinkSuggestionStore for unit tests.
type MemoryLinkSuggestionStore struct {
	mu          sync.RWMutex
	suggestions map[string]LinkSuggestion
}

// NewMemoryLinkSuggestionStore returns an empty MemoryLinkSuggestionStore.
func NewMemoryLinkSuggestionStore() *MemoryLinkSuggestionStore {
	return &MemoryLinkSuggestionStore{
		suggestions: make(map[string]LinkSuggestion),
	}
}

func (m *MemoryLinkSuggestionStore) CreateLinkSuggestion(sourceAssetID, targetAssetID, matchReason string, confidence float64) (LinkSuggestion, error) {
	now := time.Now().UTC()
	source := strings.TrimSpace(sourceAssetID)
	target := strings.TrimSpace(targetAssetID)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for duplicate (source, target) pair.
	for _, existing := range m.suggestions {
		if existing.SourceAssetID == source && existing.TargetAssetID == target {
			return existing, nil // ON CONFLICT do nothing — return existing.
		}
	}

	s := LinkSuggestion{
		ID:            idgen.New("lsug"),
		SourceAssetID: source,
		TargetAssetID: target,
		MatchReason:   strings.TrimSpace(matchReason),
		Confidence:    confidence,
		Status:        "pending",
		CreatedAt:     now,
	}
	m.suggestions[s.ID] = s
	return s, nil
}

func (m *MemoryLinkSuggestionStore) ListPendingLinkSuggestions() ([]LinkSuggestion, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]LinkSuggestion, 0, len(m.suggestions))
	for _, s := range m.suggestions {
		if s.Status == "pending" {
			out = append(out, s)
		}
	}

	// Sort by confidence DESC, then created_at DESC.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			if out[j].Confidence > out[j-1].Confidence ||
				(out[j].Confidence == out[j-1].Confidence && out[j].CreatedAt.After(out[j-1].CreatedAt)) {
				out[j], out[j-1] = out[j-1], out[j]
			}
		}
	}

	return out, nil
}

func (m *MemoryLinkSuggestionStore) ResolveLinkSuggestion(id, status, resolvedBy string) error {
	now := time.Now().UTC()
	id = strings.TrimSpace(id)
	status = strings.TrimSpace(status)

	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.suggestions[id]
	if !ok {
		return ErrNotFound
	}

	s.Status = status
	s.ResolvedAt = &now
	s.ResolvedBy = strings.TrimSpace(resolvedBy)
	m.suggestions[id] = s
	return nil
}
