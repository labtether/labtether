package persistence

import (
	"fmt"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/enrollment"
)

// MemoryEnrollmentStore provides an in-memory implementation of EnrollmentStore for testing.
type MemoryEnrollmentStore struct {
	mu               sync.RWMutex
	enrollmentTokens map[string]enrollment.EnrollmentToken // id -> token
	enrollmentByHash map[string]string                     // tokenHash -> id
	agentTokens      map[string]enrollment.AgentToken      // id -> token
	agentByHash      map[string]string                     // tokenHash -> id
	nextID           int
}

func NewMemoryEnrollmentStore() *MemoryEnrollmentStore {
	return &MemoryEnrollmentStore{
		enrollmentTokens: make(map[string]enrollment.EnrollmentToken),
		enrollmentByHash: make(map[string]string),
		agentTokens:      make(map[string]enrollment.AgentToken),
		agentByHash:      make(map[string]string),
	}
}

func (m *MemoryEnrollmentStore) CreateEnrollmentToken(tokenHash, label string, expiresAt time.Time, maxUses int) (enrollment.EnrollmentToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextID++
	now := time.Now().UTC()
	tok := enrollment.EnrollmentToken{
		ID:        fmt.Sprintf("etok-%d", m.nextID),
		Label:     label,
		ExpiresAt: expiresAt,
		MaxUses:   maxUses,
		UseCount:  0,
		CreatedAt: now,
	}
	m.enrollmentTokens[tok.ID] = tok
	m.enrollmentByHash[tokenHash] = tok.ID
	return tok, nil
}

func (m *MemoryEnrollmentStore) ValidateEnrollmentToken(tokenHash string) (enrollment.EnrollmentToken, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.enrollmentByHash[tokenHash]
	if !ok {
		return enrollment.EnrollmentToken{}, false, nil
	}
	tok := m.enrollmentTokens[id]

	now := time.Now().UTC()
	if tok.RevokedAt != nil {
		return tok, false, nil
	}
	if now.After(tok.ExpiresAt) {
		return tok, false, nil
	}
	if tok.MaxUses > 0 && tok.UseCount >= tok.MaxUses {
		return tok, false, nil
	}
	return tok, true, nil
}

func (m *MemoryEnrollmentStore) ConsumeEnrollmentToken(tokenHash string) (enrollment.EnrollmentToken, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id, ok := m.enrollmentByHash[tokenHash]
	if !ok {
		return enrollment.EnrollmentToken{}, false, nil
	}
	tok := m.enrollmentTokens[id]

	now := time.Now().UTC()
	if tok.RevokedAt != nil {
		return tok, false, nil
	}
	if now.After(tok.ExpiresAt) {
		return tok, false, nil
	}
	if tok.MaxUses > 0 && tok.UseCount >= tok.MaxUses {
		return tok, false, nil
	}

	tok.UseCount++
	m.enrollmentTokens[id] = tok
	return tok, true, nil
}

func (m *MemoryEnrollmentStore) IncrementEnrollmentTokenUse(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tok, ok := m.enrollmentTokens[id]
	if !ok {
		return fmt.Errorf("enrollment token %s not found", id)
	}
	tok.UseCount++
	m.enrollmentTokens[id] = tok
	return nil
}

func (m *MemoryEnrollmentStore) RevokeEnrollmentToken(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tok, ok := m.enrollmentTokens[id]
	if !ok {
		return fmt.Errorf("enrollment token %s not found", id)
	}
	if tok.RevokedAt != nil {
		return nil
	}
	now := time.Now().UTC()
	tok.RevokedAt = &now
	m.enrollmentTokens[id] = tok
	return nil
}

func (m *MemoryEnrollmentStore) ListEnrollmentTokens(limit int) ([]enrollment.EnrollmentToken, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}
	var tokens []enrollment.EnrollmentToken
	for _, tok := range m.enrollmentTokens {
		tokens = append(tokens, tok)
		if len(tokens) >= limit {
			break
		}
	}
	return tokens, nil
}

func (m *MemoryEnrollmentStore) CreateAgentToken(assetID, tokenHash, enrolledVia string, expiresAt time.Time) (enrollment.AgentToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextID++
	now := time.Now().UTC()
	tok := enrollment.AgentToken{
		ID:          fmt.Sprintf("atok-%d", m.nextID),
		AssetID:     assetID,
		Status:      "active",
		EnrolledVia: enrolledVia,
		ExpiresAt:   expiresAt.UTC(),
		CreatedAt:   now,
	}
	m.agentTokens[tok.ID] = tok
	m.agentByHash[tokenHash] = tok.ID
	return tok, nil
}

func (m *MemoryEnrollmentStore) ValidateAgentToken(tokenHash string) (enrollment.AgentToken, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.agentByHash[tokenHash]
	if !ok {
		return enrollment.AgentToken{}, false, nil
	}
	tok := m.agentTokens[id]
	if tok.Status != "active" {
		return tok, false, nil
	}
	if !time.Now().UTC().Before(tok.ExpiresAt) {
		return tok, false, nil
	}
	return tok, true, nil
}

func (m *MemoryEnrollmentStore) TouchAgentTokenLastUsed(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tok, ok := m.agentTokens[id]
	if !ok {
		return fmt.Errorf("agent token %s not found", id)
	}
	now := time.Now().UTC()
	tok.LastUsedAt = &now
	m.agentTokens[id] = tok
	return nil
}

func (m *MemoryEnrollmentStore) RevokeAgentToken(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tok, ok := m.agentTokens[id]
	if !ok {
		return fmt.Errorf("agent token %s not found", id)
	}
	if tok.Status != "active" {
		return nil
	}
	now := time.Now().UTC()
	tok.Status = "revoked"
	tok.RevokedAt = &now
	m.agentTokens[id] = tok
	return nil
}

func (m *MemoryEnrollmentStore) RevokeAgentTokensByAsset(assetID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	for id, tok := range m.agentTokens {
		if tok.AssetID == assetID && tok.Status == "active" {
			tok.Status = "revoked"
			tok.RevokedAt = &now
			m.agentTokens[id] = tok
		}
	}
	return nil
}

func (m *MemoryEnrollmentStore) DeleteDeadTokens() (int, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	enrollDeleted := 0
	for id, tok := range m.enrollmentTokens {
		isDead := tok.RevokedAt != nil ||
			now.After(tok.ExpiresAt) ||
			(tok.MaxUses > 0 && tok.UseCount >= tok.MaxUses)
		if isDead {
			delete(m.enrollmentTokens, id)
			for hash, hid := range m.enrollmentByHash {
				if hid == id {
					delete(m.enrollmentByHash, hash)
				}
			}
			enrollDeleted++
		}
	}

	agentDeleted := 0
	for id, tok := range m.agentTokens {
		if tok.Status == "revoked" && tok.LastUsedAt == nil {
			delete(m.agentTokens, id)
			for hash, hid := range m.agentByHash {
				if hid == id {
					delete(m.agentByHash, hash)
				}
			}
			agentDeleted++
		}
	}

	return enrollDeleted, agentDeleted, nil
}

func (m *MemoryEnrollmentStore) ListAgentTokens(limit int) ([]enrollment.AgentToken, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}
	var tokens []enrollment.AgentToken
	for _, tok := range m.agentTokens {
		tokens = append(tokens, tok)
		if len(tokens) >= limit {
			break
		}
	}
	return tokens, nil
}
