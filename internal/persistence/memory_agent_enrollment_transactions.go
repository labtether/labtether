package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/enrollment"
)

func (m *MemoryEnrollmentStore) CommitAgentEnrollment(ctx context.Context, req AgentEnrollmentCommitRequest) (AgentEnrollmentCommitResult, error) {
	if err := ctx.Err(); err != nil {
		return AgentEnrollmentCommitResult{}, err
	}
	assetID := strings.TrimSpace(req.AssetID)
	if assetID == "" {
		return AgentEnrollmentCommitResult{}, fmt.Errorf("asset id is required")
	}
	req.AssetID = assetID
	if m.assetStore == nil {
		return AgentEnrollmentCommitResult{}, ErrAgentEnrollmentTransactionsUnavailable
	}

	// Lock order is group -> asset -> enrollment everywhere that spans these
	// stores. Holding the group read lock through the critical section keeps a
	// validated placement from disappearing before the asset is committed.
	if m.groupStore != nil {
		m.groupStore.mu.RLock()
		defer m.groupStore.mu.RUnlock()
	}
	m.assetStore.mu.Lock()
	defer m.assetStore.mu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	if !now.Before(req.AgentTokenExpiresAt) {
		return AgentEnrollmentCommitResult{}, fmt.Errorf("token expiry must be in the future")
	}
	etok, valid := m.validateEnrollmentTokenLocked(req.EnrollmentTokenHash, now)
	if !valid {
		return AgentEnrollmentCommitResult{}, ErrEnrollmentTokenInvalid
	}
	if _, duplicate := m.agentByHash[req.AgentTokenHash]; duplicate {
		return AgentEnrollmentCommitResult{}, fmt.Errorf("agent token hash already exists")
	}

	existing, exists := m.assetStore.assets[req.AssetID]
	if exists {
		if strings.TrimSpace(req.DeviceProofVersion) != enrollment.DeviceProofVersionV2 {
			return AgentEnrollmentCommitResult{}, ErrAgentIdentityProofV2Required
		}
		if etok.MaxUses != 1 {
			return AgentEnrollmentCommitResult{}, ErrRecoveryRequiresSingleUseToken
		}
		storedFingerprint := strings.TrimSpace(existing.Metadata[assets.MetadataKeyAgentDeviceFingerprint])
		storedAlgorithm := strings.TrimSpace(existing.Metadata[assets.MetadataKeyAgentDeviceKeyAlgorithm])
		if storedFingerprint == "" || storedAlgorithm == "" ||
			storedFingerprint != strings.TrimSpace(req.DeviceFingerprint) ||
			storedAlgorithm != strings.TrimSpace(req.DeviceKeyAlgorithm) {
			return AgentEnrollmentCommitResult{}, ErrAgentIdentityContinuityConflict
		}
		marker := m.identityMarkerLocked(req.AssetID, existing)
		if !etok.CreatedAt.After(marker) {
			return AgentEnrollmentCommitResult{}, ErrEnrollmentTokenPredatesRotation
		}
	} else {
		if err := validateInitialIdentityFields(req); err != nil {
			return AgentEnrollmentCommitResult{}, err
		}
		if m.enrolledFleetCardinalityLocked(now) >= normalizedMaxEnrolledAgents(req.MaxEnrolledAgents) {
			return AgentEnrollmentCommitResult{}, ErrAgentFleetCapacityReached
		}
		req.GroupID = m.resolveInitialEnrollmentGroupIDLocked(req.GroupID)
	}

	// All validation is complete. Mutate both stores as one critical section.
	etok.UseCount++
	m.enrollmentTokens[etok.ID] = etok

	for id, token := range m.agentTokens {
		if token.AssetID == req.AssetID && token.Status == "active" {
			token.Status = "revoked"
			token.RevokedAt = timePointer(now)
			m.agentTokens[id] = token
		}
	}
	m.nextID++
	agentToken := enrollment.AgentToken{
		ID:          fmt.Sprintf("atok-%d", m.nextID),
		AssetID:     req.AssetID,
		Status:      "active",
		EnrolledVia: etok.ID,
		ExpiresAt:   req.AgentTokenExpiresAt.UTC(),
		CreatedAt:   now,
	}
	m.agentTokens[agentToken.ID] = agentToken
	m.agentByHash[req.AgentTokenHash] = agentToken.ID

	if !exists {
		existing = buildInitialEnrolledAsset(req, now)
		m.assetStore.assets[req.AssetID] = existing
	}
	m.identityRotatedAt[req.AssetID] = now
	existing = cloneAssetForReturn(existing)
	return AgentEnrollmentCommitResult{
		EnrollmentToken: etok,
		AgentToken:      agentToken,
		Asset:           existing,
		Recovery:        exists,
	}, nil
}

func (m *MemoryEnrollmentStore) resolveInitialEnrollmentGroupIDLocked(groupID string) string {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" || m.groupStore == nil {
		return groupID
	}
	if _, exists := m.groupStore.groups[groupID]; !exists {
		return ""
	}
	return groupID
}

func (m *MemoryEnrollmentStore) PrepareAgentApproval(ctx context.Context, req AgentApprovalPrepareRequest) (enrollment.AgentToken, error) {
	if err := ctx.Err(); err != nil {
		return enrollment.AgentToken{}, err
	}
	assetID := strings.TrimSpace(req.AssetID)
	if assetID == "" {
		return enrollment.AgentToken{}, fmt.Errorf("asset id is required")
	}
	if m.assetStore == nil {
		return enrollment.AgentToken{}, ErrAgentEnrollmentTransactionsUnavailable
	}
	m.assetStore.mu.Lock()
	defer m.assetStore.mu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.assetStore.assets[assetID]; exists {
		return enrollment.AgentToken{}, ErrAgentApprovalAssetConflict
	}
	if _, duplicate := m.agentByHash[req.AgentTokenHash]; duplicate {
		return enrollment.AgentToken{}, fmt.Errorf("agent token hash already exists")
	}
	now := time.Now().UTC()
	for _, existing := range m.agentTokens {
		if existing.AssetID == assetID && existing.Status == "pending" && now.Before(existing.ExpiresAt) {
			return enrollment.AgentToken{}, ErrAgentApprovalAssetConflict
		}
	}
	if m.enrolledFleetCardinalityLocked(now) >= normalizedMaxEnrolledAgents(req.MaxEnrolledAgents) {
		return enrollment.AgentToken{}, ErrAgentFleetCapacityReached
	}
	m.nextID++
	token := enrollment.AgentToken{
		ID:          fmt.Sprintf("atok-%d", m.nextID),
		AssetID:     assetID,
		Status:      "pending",
		EnrolledVia: "console-approval",
		ExpiresAt:   boundedPreparedApprovalExpiry(req.PreparedTokenExpiresAt, now),
		CreatedAt:   now,
	}
	m.agentTokens[token.ID] = token
	m.agentByHash[req.AgentTokenHash] = token.ID
	return token, nil
}

func (m *MemoryEnrollmentStore) FinalizeAgentApproval(ctx context.Context, req AgentApprovalFinalizeRequest) (assets.Asset, error) {
	if err := ctx.Err(); err != nil {
		return assets.Asset{}, err
	}
	assetID := strings.TrimSpace(req.AssetID)
	if assetID == "" {
		return assets.Asset{}, ErrAgentIdentityContinuityConflict
	}
	req.AssetID = assetID
	if m.assetStore == nil {
		return assets.Asset{}, ErrAgentEnrollmentTransactionsUnavailable
	}
	m.assetStore.mu.Lock()
	defer m.assetStore.mu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	prepared, ok := m.agentTokens[strings.TrimSpace(req.PreparedTokenID)]
	if !ok || prepared.Status != "pending" || prepared.RevokedAt != nil || prepared.AssetID != assetID ||
		!now.Before(prepared.ExpiresAt) {
		return assets.Asset{}, ErrPreparedAgentApprovalNotFound
	}
	if !now.Before(req.AgentTokenExpiresAt) {
		return assets.Asset{}, fmt.Errorf("token expiry must be in the future")
	}
	fingerprint := strings.TrimSpace(req.DeviceFingerprint)
	algorithm := strings.TrimSpace(req.DeviceKeyAlgorithm)
	if fingerprint == "" || algorithm == "" {
		return assets.Asset{}, ErrAgentIdentityContinuityConflict
	}

	asset, exists := m.assetStore.assets[req.AssetID]
	if exists {
		return assets.Asset{}, ErrAgentApprovalAssetConflict
	} else {
		asset = assets.Asset{
			ID:         req.AssetID,
			Type:       "node",
			Name:       req.Hostname,
			Source:     "agent",
			Status:     "pending",
			Platform:   req.Platform,
			Metadata:   map[string]string{},
			CreatedAt:  now,
			LastSeenAt: now,
		}
	}
	asset.Metadata[assets.MetadataKeyAgentDeviceFingerprint] = fingerprint
	asset.Metadata[assets.MetadataKeyAgentDeviceKeyAlgorithm] = algorithm
	asset.Metadata[assets.MetadataKeyAgentIdentityVerifiedAt] = now.Format(time.RFC3339Nano)
	asset.UpdatedAt = now
	asset = applyAssetCanonical(asset)

	for id, token := range m.agentTokens {
		if token.AssetID == req.AssetID && token.Status == "active" {
			token.Status = "revoked"
			token.RevokedAt = timePointer(now)
			m.agentTokens[id] = token
		}
	}
	prepared.Status = "active"
	prepared.ExpiresAt = req.AgentTokenExpiresAt.UTC()
	m.agentTokens[prepared.ID] = prepared
	m.assetStore.assets[asset.ID] = asset
	m.identityRotatedAt[asset.ID] = now
	return cloneAssetForReturn(asset), nil
}

func (m *MemoryEnrollmentStore) CancelAgentApproval(ctx context.Context, preparedTokenID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	token, ok := m.agentTokens[strings.TrimSpace(preparedTokenID)]
	if !ok || token.Status != "pending" {
		return nil
	}
	now := time.Now().UTC()
	token.Status = "revoked"
	token.RevokedAt = &now
	m.agentTokens[token.ID] = token
	return nil
}

func (m *MemoryEnrollmentStore) DecommissionAgentAsset(ctx context.Context, assetID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.assetStore == nil {
		return ErrAgentEnrollmentTransactionsUnavailable
	}
	m.assetStore.mu.Lock()
	defer m.assetStore.mu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()

	assetID = strings.TrimSpace(assetID)
	if _, ok := m.assetStore.assets[assetID]; !ok {
		return ErrNotFound
	}
	now := time.Now().UTC()
	for id, token := range m.agentTokens {
		if token.AssetID == assetID && (token.Status == "active" || token.Status == "pending") {
			token.Status = "revoked"
			token.RevokedAt = timePointer(now)
			m.agentTokens[id] = token
		}
	}
	delete(m.assetStore.assets, assetID)
	delete(m.identityRotatedAt, assetID)
	return nil
}

func (m *MemoryEnrollmentStore) CommitAuthenticatedAgentHeartbeat(ctx context.Context, agentTokenID string, req assets.HeartbeatRequest) (assets.Asset, error) {
	if err := ctx.Err(); err != nil {
		return assets.Asset{}, err
	}
	assetID := strings.TrimSpace(req.AssetID)
	if assetID == "" {
		return assets.Asset{}, ErrAgentCredentialInactive
	}
	req.AssetID = assetID
	if m.assetStore == nil {
		return assets.Asset{}, ErrAgentEnrollmentTransactionsUnavailable
	}
	m.assetStore.mu.Lock()
	defer m.assetStore.mu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	token, ok := m.agentTokens[strings.TrimSpace(agentTokenID)]
	now := time.Now().UTC()
	if !ok || token.Status != "active" || token.RevokedAt != nil || !now.Before(token.ExpiresAt) || token.AssetID != assetID {
		return assets.Asset{}, ErrAgentCredentialInactive
	}
	existing, exists := m.assetStore.assets[assetID]
	if !exists {
		return assets.Asset{}, ErrAgentCredentialInactive
	}
	lastUsedAt := now
	token.LastUsedAt = &lastUsedAt
	m.agentTokens[token.ID] = token
	req.AllowAgentIdentityTOFU = true
	req.Source = "agent"
	req.GroupID = existing.GroupID
	return m.assetStore.upsertAssetHeartbeatLocked(req, now), nil
}

func (m *MemoryEnrollmentStore) CommitExistingOwnerAgentHeartbeat(ctx context.Context, req assets.HeartbeatRequest) (assets.Asset, error) {
	if err := ctx.Err(); err != nil {
		return assets.Asset{}, err
	}
	assetID := strings.TrimSpace(req.AssetID)
	if assetID == "" {
		return assets.Asset{}, ErrNotFound
	}
	req.AssetID = assetID
	if m.assetStore == nil {
		return assets.Asset{}, ErrAgentEnrollmentTransactionsUnavailable
	}
	m.assetStore.mu.Lock()
	defer m.assetStore.mu.Unlock()
	existing, exists := m.assetStore.assets[assetID]
	if !exists {
		return assets.Asset{}, ErrNotFound
	}
	req.Source = "agent"
	req.GroupID = existing.GroupID
	return m.assetStore.upsertAssetHeartbeatLocked(req, time.Now().UTC()), nil
}

func (m *MemoryEnrollmentStore) ValidateActiveAgentTokenID(ctx context.Context, agentTokenID, assetID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.assetStore == nil {
		return ErrAgentEnrollmentTransactionsUnavailable
	}
	// Preserve the global lock order used by enrollment, decommission, and
	// authenticated heartbeat transactions: asset store before token store.
	m.assetStore.mu.RLock()
	defer m.assetStore.mu.RUnlock()
	m.mu.RLock()
	defer m.mu.RUnlock()
	assetID = strings.TrimSpace(assetID)
	token, ok := m.agentTokens[strings.TrimSpace(agentTokenID)]
	_, assetExists := m.assetStore.assets[assetID]
	if !ok || !assetExists || token.Status != "active" || token.RevokedAt != nil || !time.Now().UTC().Before(token.ExpiresAt) || token.AssetID != assetID {
		return ErrAgentCredentialInactive
	}
	return nil
}

func (m *MemoryEnrollmentStore) validateEnrollmentTokenLocked(tokenHash string, now time.Time) (enrollment.EnrollmentToken, bool) {
	id, ok := m.enrollmentByHash[tokenHash]
	if !ok {
		return enrollment.EnrollmentToken{}, false
	}
	token := m.enrollmentTokens[id]
	if token.RevokedAt != nil || !now.Before(token.ExpiresAt) || token.MaxUses < 1 ||
		token.MaxUses > enrollment.HardTokenMaxUsesCeiling || token.UseCount >= token.MaxUses {
		return token, false
	}
	return token, true
}

func normalizedMaxEnrolledAgents(value int) int {
	return enrollment.BoundedLimit(value, enrollment.DefaultMaxEnrolledAgents, enrollment.HardMaxEnrolledAgents)
}

// enrolledFleetCardinalityLocked counts durable enrolled identities even when
// every bearer has expired or been revoked. Only explicit decommission removes
// an identity-state row. Live approval reservations consume capacity too.
func (m *MemoryEnrollmentStore) enrolledFleetCardinalityLocked(now time.Time) int {
	identities := make(map[string]struct{}, len(m.identityRotatedAt))
	for assetID := range m.identityRotatedAt {
		identities[assetID] = struct{}{}
	}
	if m.assetStore != nil {
		for assetID, asset := range m.assetStore.assets {
			if strings.EqualFold(strings.TrimSpace(asset.Source), "agent") {
				identities[assetID] = struct{}{}
			}
		}
	}
	for _, token := range m.agentTokens {
		if token.Status == "pending" && now.Before(token.ExpiresAt) {
			identities[token.AssetID] = struct{}{}
		}
	}
	return len(identities)
}

func (m *MemoryEnrollmentStore) identityMarkerLocked(assetID string, asset assets.Asset) time.Time {
	if marker, ok := m.identityRotatedAt[assetID]; ok {
		return marker.UTC()
	}
	marker := asset.CreatedAt.UTC()
	for _, token := range m.agentTokens {
		if token.AssetID == assetID && token.CreatedAt.After(marker) {
			marker = token.CreatedAt.UTC()
		}
	}
	return marker
}

func validateInitialIdentityFields(req AgentEnrollmentCommitRequest) error {
	fingerprint := strings.TrimSpace(req.DeviceFingerprint)
	algorithm := strings.TrimSpace(req.DeviceKeyAlgorithm)
	version := strings.TrimSpace(req.DeviceProofVersion)
	provided := fingerprint != "" || algorithm != "" || version != ""
	if !provided {
		return nil
	}
	if fingerprint == "" || algorithm == "" || (version != enrollment.DeviceProofVersionV1 && version != enrollment.DeviceProofVersionV2) {
		return ErrAgentIdentityContinuityConflict
	}
	return nil
}

func buildInitialEnrolledAsset(req AgentEnrollmentCommitRequest, now time.Time) assets.Asset {
	metadata := map[string]string{}
	if strings.TrimSpace(req.DeviceFingerprint) != "" {
		metadata[assets.MetadataKeyAgentDeviceFingerprint] = strings.TrimSpace(req.DeviceFingerprint)
		metadata[assets.MetadataKeyAgentDeviceKeyAlgorithm] = strings.TrimSpace(req.DeviceKeyAlgorithm)
		metadata[assets.MetadataKeyAgentIdentityVerifiedAt] = now.Format(time.RFC3339Nano)
	}
	asset := assets.Asset{
		ID:         req.AssetID,
		Type:       "node",
		Name:       req.Hostname,
		Source:     "agent",
		GroupID:    strings.TrimSpace(req.GroupID),
		Status:     "online",
		Platform:   req.Platform,
		Metadata:   metadata,
		CreatedAt:  now,
		UpdatedAt:  now,
		LastSeenAt: now,
	}
	return applyAssetCanonical(asset)
}

func cloneAssetForReturn(asset assets.Asset) assets.Asset {
	asset.Metadata = cloneMetadata(asset.Metadata)
	asset.Tags = cloneStringSlice(asset.Tags)
	asset.Attributes = cloneAnyMap(asset.Attributes)
	return asset
}

func timePointer(value time.Time) *time.Time {
	value = value.UTC()
	return &value
}
