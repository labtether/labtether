package persistence

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/enrollment"
)

// MemoryEnrollmentStore provides an in-memory implementation of EnrollmentStore for testing.
type MemoryEnrollmentStore struct {
	mu                sync.RWMutex
	enrollmentTokens  map[string]enrollment.EnrollmentToken // id -> token
	enrollmentByHash  map[string]string                     // tokenHash -> id
	agentTokens       map[string]enrollment.AgentToken      // id -> token
	agentByHash       map[string]string                     // tokenHash -> id
	identityRotatedAt map[string]time.Time                  // assetID -> latest durable credential rotation
	nextID            int
	assetStore        *MemoryAssetStore
	groupStore        *MemoryGroupStore
}

func NewMemoryEnrollmentStore(assetStores ...*MemoryAssetStore) *MemoryEnrollmentStore {
	store := &MemoryEnrollmentStore{
		enrollmentTokens:  make(map[string]enrollment.EnrollmentToken),
		enrollmentByHash:  make(map[string]string),
		agentTokens:       make(map[string]enrollment.AgentToken),
		agentByHash:       make(map[string]string),
		identityRotatedAt: make(map[string]time.Time),
	}
	if len(assetStores) > 0 {
		store.assetStore = assetStores[0]
	}
	return store
}

// NewMemoryEnrollmentStoreWithGroupStore enables the same initial enrollment
// placement validation enforced by Postgres. Tests that exercise HTTP
// enrollment should use this constructor so a stale group id cannot create an
// in-memory state that production would reject at the foreign-key boundary.
func NewMemoryEnrollmentStoreWithGroupStore(assetStore *MemoryAssetStore, groupStore *MemoryGroupStore) *MemoryEnrollmentStore {
	store := NewMemoryEnrollmentStore(assetStore)
	store.groupStore = groupStore
	return store
}

func (m *MemoryEnrollmentStore) CreateEnrollmentToken(tokenHash, label string, expiresAt time.Time, maxUses int) (enrollment.EnrollmentToken, error) {
	if err := enrollment.ValidateStoredTokenMaxUses(maxUses); err != nil {
		return enrollment.EnrollmentToken{}, err
	}
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
	if !now.Before(tok.ExpiresAt) {
		return tok, false, nil
	}
	if tok.MaxUses < 1 || tok.MaxUses > enrollment.HardTokenMaxUsesCeiling || tok.UseCount >= tok.MaxUses {
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
	if !now.Before(tok.ExpiresAt) {
		return tok, false, nil
	}
	if tok.MaxUses < 1 || tok.MaxUses > enrollment.HardTokenMaxUsesCeiling || tok.UseCount >= tok.MaxUses {
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
		return ErrEnrollmentTokenInvalid
	}
	now := time.Now().UTC()
	if tok.RevokedAt != nil || !now.Before(tok.ExpiresAt) || tok.MaxUses < 1 || tok.MaxUses > enrollment.HardTokenMaxUsesCeiling || tok.UseCount >= tok.MaxUses {
		return ErrEnrollmentTokenInvalid
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
		return ErrNotFound
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
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return enrollment.AgentToken{}, fmt.Errorf("asset id is required")
	}
	if m.assetStore != nil {
		m.assetStore.mu.Lock()
		defer m.assetStore.mu.Unlock()
	}
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
	if m.assetStore != nil {
		if _, exists := m.assetStore.assets[assetID]; exists {
			m.identityRotatedAt[assetID] = now
		}
	}
	return tok, nil
}

func (m *MemoryEnrollmentStore) RotateAgentToken(assetID, tokenHash, enrolledVia string, expiresAt time.Time) (enrollment.AgentToken, error) {
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return enrollment.AgentToken{}, fmt.Errorf("asset id is required")
	}
	if m.assetStore != nil {
		m.assetStore.mu.Lock()
		defer m.assetStore.mu.Unlock()
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	for id, existing := range m.agentTokens {
		if existing.AssetID == assetID && existing.Status == "active" {
			existing.Status = "revoked"
			existing.RevokedAt = &now
			m.agentTokens[id] = existing
		}
	}

	m.nextID++
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
	if m.assetStore != nil {
		if _, exists := m.assetStore.assets[assetID]; exists {
			m.identityRotatedAt[assetID] = now
		}
	}
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
	if tok.Status != "active" || tok.RevokedAt != nil {
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
		return ErrNotFound
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
			!now.Before(tok.ExpiresAt) ||
			tok.MaxUses < 1 ||
			tok.MaxUses > enrollment.HardTokenMaxUsesCeiling ||
			tok.UseCount >= tok.MaxUses
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
		if (tok.Status == "revoked" && tok.LastUsedAt == nil) || (tok.Status == "pending" && !now.Before(tok.ExpiresAt)) {
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
