package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

const (
	sshHubKeyRotationConfirmation = "ROTATE"
	maxSSHHubKeyRotationBodyBytes = 4096
	maxSSHHubKeyRotationReasonLen = 256
	sshHubKeyRotationAuditType    = "settings.ssh_hub_key.rotation"
)

type sshKeyRotateRequest struct {
	KeyType string `json:"key_type"`
	Reason  string `json:"reason"`
	Confirm string `json:"confirm"`
}

func normalizeSSHHubKeyRotationRequest(req sshKeyRotateRequest, currentKeyType string) (sshKeyRotateRequest, error) {
	req.KeyType = strings.ToLower(strings.TrimSpace(req.KeyType))
	rawReason := req.Reason
	if req.Confirm != sshHubKeyRotationConfirmation {
		return sshKeyRotateRequest{}, fmt.Errorf("confirmation required: type %s", sshHubKeyRotationConfirmation)
	}
	if len(rawReason) > maxSSHHubKeyRotationReasonLen*utf8.UTFMax || utf8.RuneCountInString(rawReason) > maxSSHHubKeyRotationReasonLen {
		return sshKeyRotateRequest{}, fmt.Errorf("reason must be at most %d characters", maxSSHHubKeyRotationReasonLen)
	}
	for _, character := range rawReason {
		if unicode.IsControl(character) {
			return sshKeyRotateRequest{}, fmt.Errorf("reason must not contain control characters")
		}
	}
	req.Reason = strings.TrimSpace(rawReason)
	if req.KeyType == "" {
		req.KeyType = currentKeyType
	}
	keyType, err := normalizeHubSSHKeyType(req.KeyType)
	if err != nil {
		return sshKeyRotateRequest{}, fmt.Errorf("key_type must be ed25519 or rsa")
	}
	req.KeyType = keyType
	if req.Reason == "" {
		req.Reason = "manual rotation"
	}
	return req, nil
}

func decodeSSHHubKeyRotationRequest(w http.ResponseWriter, r *http.Request, dst *sshKeyRotateRequest) error {
	if r == nil || r.Body == nil {
		return nil
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxSSHHubKeyRotationBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("request body must contain one JSON object")
	}
	return nil
}

func (s *apiServer) validatePersistedHubSSHIdentity(identity *hubSSHIdentity) error {
	privateKeyPEM, err := s.loadHubPrivateKeyPEM(identity)
	if err != nil {
		return err
	}
	signer, err := ssh.ParsePrivateKey([]byte(privateKeyPEM))
	if err != nil {
		return fmt.Errorf("failed to parse persisted hub SSH private key: %w", err)
	}
	persistedPublicKey := string(ssh.MarshalAuthorizedKey(signer.PublicKey()))
	if strings.TrimSpace(persistedPublicKey) != strings.TrimSpace(identity.PublicKey) {
		return fmt.Errorf("persisted hub SSH key does not match active identity")
	}
	persistedKeyType, err := hubSSHKeyTypeFromPublicKey(signer.PublicKey())
	if err != nil {
		return err
	}
	if persistedKeyType != identity.KeyType {
		return fmt.Errorf("persisted hub SSH key type does not match active identity")
	}
	return nil
}

func sendHubSSHKeyMessage(conn *agentmgr.AgentConn, messageType, publicKey string) error {
	if conn == nil {
		return fmt.Errorf("agent connection unavailable")
	}
	var payload any
	switch messageType {
	case agentmgr.MsgSSHKeyInstall:
		payload = agentmgr.SSHKeyInstallData{PublicKey: publicKey}
	case agentmgr.MsgSSHKeyRemove:
		payload = agentmgr.SSHKeyRemoveData{PublicKey: publicKey}
	default:
		return fmt.Errorf("unsupported SSH key message type")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode SSH key message: %w", err)
	}
	return conn.Send(agentmgr.Message{Type: messageType, Data: data})
}

func (s *apiServer) stageHubSSHKey(publicKey string) (connectedAssets, stagedAssets []string, failures int) {
	if s.agentMgr == nil {
		return nil, nil, 0
	}
	connectedAssets = s.agentMgr.ConnectedAssets()
	sort.Strings(connectedAssets)
	stagedAssets = make([]string, 0, len(connectedAssets))
	for _, assetID := range connectedAssets {
		conn, ok := s.agentMgr.Get(assetID)
		if !ok {
			failures++
			continue
		}
		if err := sendHubSSHKeyMessage(conn, agentmgr.MsgSSHKeyInstall, publicKey); err != nil {
			failures++
			log.Printf("ssh-rotate: failed to stage key on asset %s: %v", assetID, err) // #nosec G706 -- Asset IDs are server-controlled and no key material is logged.
			continue
		}
		stagedAssets = append(stagedAssets, assetID)
	}
	return connectedAssets, stagedAssets, failures
}

func (s *apiServer) removeHubSSHKeyFromAssets(assetIDs []string, publicKey string) int {
	if s.agentMgr == nil {
		return 0
	}
	failures := 0
	for _, assetID := range assetIDs {
		conn, ok := s.agentMgr.Get(assetID)
		if !ok {
			failures++
			continue
		}
		if err := sendHubSSHKeyMessage(conn, agentmgr.MsgSSHKeyRemove, publicKey); err != nil {
			failures++
			log.Printf("ssh-rotate: failed to remove superseded key from asset %s: %v", assetID, err) // #nosec G706 -- Asset IDs are server-controlled and no key material is logged.
		}
	}
	return failures
}

func (s *apiServer) auditSSHHubKeyRotation(
	r *http.Request,
	decision string,
	reason string,
	stage string,
	oldInfo sshHubKeyInfo,
	newInfo sshHubKeyInfo,
	agentsTotal int,
	agentsStaged int,
	failures int,
) {
	event := audit.NewEvent(sshHubKeyRotationAuditType)
	event.ActorID = principalActorID(r.Context())
	event.Target = "hub"
	event.Decision = decision
	event.Reason = reason
	event.Details = map[string]any{
		"stage":                  stage,
		"key_type":               newInfo.KeyType,
		"old_fingerprint_sha256": oldInfo.FingerprintSHA256,
		"new_fingerprint_sha256": newInfo.FingerprintSHA256,
		"agents_total":           agentsTotal,
		"agents_staged":          agentsStaged,
		"failures":               failures,
	}
	s.appendAuditEventBestEffort(event, "api warning: failed to append hub SSH key rotation audit event")
}

// handleSSHHubKeyRotate handles POST /settings/ssh-hub-key/rotate.
// Rotation is serialized with other hub-key provisioning operations. The new
// public key is staged on every currently connected agent before the only
// stored private key is replaced, so a delivery failure cannot lock the hub
// out of agents that still trust the old key.
func (s *apiServer) handleSSHHubKeyRotate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.enforceRateLimit(w, r, "ssh.key.rotate", 5, time.Minute) {
		return
	}

	s.hubIdentityOperationMu.Lock()
	defer s.hubIdentityOperationMu.Unlock()

	identity := s.currentHubSSHIdentity()
	if identity == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "hub SSH identity not configured")
		return
	}
	if s.secretsManager == nil || s.credentialStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "hub SSH key storage is unavailable")
		return
	}
	oldInfo, err := hubSSHKeyInfoForIdentity(identity)
	if err != nil || s.validatePersistedHubSSHIdentity(identity) != nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "hub SSH identity is inconsistent; rotation was not attempted")
		return
	}

	var requestPayload sshKeyRotateRequest
	if err := decodeSSHHubKeyRotationRequest(w, r, &requestPayload); err != nil {
		servicehttp.WriteError(w, shared.JSONDecodeErrorStatus(err), "invalid JSON payload")
		return
	}
	requestPayload, err = normalizeSSHHubKeyRotationRequest(requestPayload, oldInfo.KeyType)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	privateKeyPEM, publicKey, err := generateHubSSHKeypair(requestPayload.KeyType)
	if err != nil {
		s.auditSSHHubKeyRotation(r, "error", requestPayload.Reason, "generate", oldInfo, sshHubKeyInfo{KeyType: requestPayload.KeyType}, 0, 0, 1)
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to generate new hub SSH key")
		log.Printf("ssh-rotate: key generation failed for type %s: %v", requestPayload.KeyType, err)
		return
	}
	newIdentity := &hubSSHIdentity{ProfileID: identity.ProfileID, PublicKey: publicKey, KeyType: requestPayload.KeyType}
	newInfo, err := hubSSHKeyInfoForIdentity(newIdentity)
	if err != nil {
		s.auditSSHHubKeyRotation(r, "error", requestPayload.Reason, "validate", oldInfo, sshHubKeyInfo{KeyType: requestPayload.KeyType}, 0, 0, 1)
		servicehttp.WriteError(w, http.StatusInternalServerError, "generated hub SSH key failed validation")
		return
	}
	ciphertext, err := s.secretsManager.EncryptString(string(privateKeyPEM), identity.ProfileID)
	if err != nil {
		s.auditSSHHubKeyRotation(r, "error", requestPayload.Reason, "encrypt", oldInfo, newInfo, 0, 0, 1)
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to protect new hub SSH key")
		log.Printf("ssh-rotate: encryption failed for profile %s: %v", identity.ProfileID, err) // #nosec G706 -- Profile IDs are server-generated; no key material is logged.
		return
	}

	connectedAssets, stagedAssets, stagingFailures := s.stageHubSSHKey(publicKey)
	if stagingFailures > 0 {
		rollbackFailures := s.removeHubSSHKeyFromAssets(stagedAssets, publicKey)
		s.auditSSHHubKeyRotation(r, "error", requestPayload.Reason, "stage", oldInfo, newInfo, len(connectedAssets), len(stagedAssets), stagingFailures+rollbackFailures)
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to stage the new hub SSH key on all connected agents; the existing key remains active")
		return
	}

	if _, err := s.credentialStore.UpdateCredentialProfileSecret(identity.ProfileID, ciphertext, "", nil); err != nil {
		rollbackFailures := s.removeHubSSHKeyFromAssets(stagedAssets, publicKey)
		s.auditSSHHubKeyRotation(r, "error", requestPayload.Reason, "persist", oldInfo, newInfo, len(connectedAssets), len(stagedAssets), 1+rollbackFailures)
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to persist new hub SSH key; the existing key remains active")
		log.Printf("ssh-rotate: credential update failed for profile %s: %v", identity.ProfileID, err) // #nosec G706 -- Profile IDs are server-generated; no key material is logged.
		return
	}

	s.setHubSSHIdentity(newIdentity)
	removalFailures := s.removeHubSSHKeyFromAssets(stagedAssets, identity.PublicKey)
	s.auditSSHHubKeyRotation(r, "applied", requestPayload.Reason, "complete", oldInfo, newInfo, len(connectedAssets), len(stagedAssets), removalFailures)

	actorID := principalActorID(r.Context())
	log.Printf("ssh-rotate: actor=%s profile=%s key_type=%s staged=%d total=%d old_key_removal_failures=%d", actorID, identity.ProfileID, newInfo.KeyType, len(stagedAssets), len(connectedAssets), removalFailures) // #nosec G706 -- Values are server-derived bounded metadata; request reason and key material are intentionally omitted.

	response := map[string]any{
		"status":                   "rotated",
		"key_type":                 newInfo.KeyType,
		"fingerprint_sha256":       newInfo.FingerprintSHA256,
		"public_key":               newInfo.PublicKey,
		"agents_updated":           len(stagedAssets),
		"agents_total":             len(connectedAssets),
		"old_key_removal_failures": removalFailures,
	}
	if removalFailures > 0 {
		response["warning"] = "the new key is active, but one or more agents may still retain the previous public key"
	}
	w.Header().Set("Cache-Control", "no-store")
	servicehttp.WriteJSON(w, http.StatusOK, response)
}
