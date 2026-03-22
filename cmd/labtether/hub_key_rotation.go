package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

type sshKeyRotateRequest struct {
	KeyType string `json:"key_type"` // "ed25519" (default) or "rsa"
	Reason  string `json:"reason"`   // optional audit note
}

// handleSSHHubKeyRotate handles POST /settings/ssh-hub-key/rotate.
// It generates a new SSH keypair, updates the credential profile, and pushes
// the new key to all connected agents.
func (s *apiServer) handleSSHHubKeyRotate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.hubIdentity == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "hub SSH identity not configured")
		return
	}
	if s.secretsManager == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "encryption not configured")
		return
	}
	if !s.enforceRateLimit(w, r, "ssh.key.rotate", 5, time.Minute) {
		return
	}

	var req sshKeyRotateRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := shared.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid JSON payload")
			return
		}
	}

	keyType := req.KeyType
	if keyType == "" {
		keyType = s.hubIdentity.KeyType
	}
	if keyType == "" {
		keyType = "ed25519"
	}
	if keyType != "ed25519" && keyType != "rsa" {
		servicehttp.WriteError(w, http.StatusBadRequest, "key_type must be ed25519 or rsa")
		return
	}

	oldPublicKey := s.hubIdentity.PublicKey
	profileID := s.hubIdentity.ProfileID

	// Generate new keypair.
	privPEM, pubKeyStr, err := generateHubSSHKeypair(keyType)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to generate new keypair")
		log.Printf("ssh-rotate: keypair generation failed: %v", err)
		return
	}

	// Encrypt new private key.
	ciphertext, err := s.secretsManager.EncryptString(string(privPEM), profileID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to encrypt new key")
		log.Printf("ssh-rotate: encryption failed: %v", err)
		return
	}

	// Update credential profile with new secret.
	if _, err := s.credentialStore.UpdateCredentialProfileSecret(profileID, ciphertext, "", nil); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update credential profile")
		log.Printf("ssh-rotate: credential update failed: %v", err)
		return
	}

	// Update in-memory identity.
	s.hubIdentity.PublicKey = pubKeyStr
	s.hubIdentity.KeyType = keyType

	// Push new key to all connected agents.
	connectedAssets := s.agentMgr.ConnectedAssets()
	pushed := 0
	for _, assetID := range connectedAssets {
		conn, ok := s.agentMgr.Get(assetID)
		if !ok {
			continue
		}
		// Install new key.
		data, _ := json.Marshal(agentmgr.SSHKeyInstallData{PublicKey: pubKeyStr})
		if err := conn.Send(agentmgr.Message{Type: agentmgr.MsgSSHKeyInstall, Data: data}); err != nil {
			log.Printf("ssh-rotate: failed to push new key to %s: %v", assetID, err) // #nosec G706 -- Asset IDs are hub-generated identifiers.
			continue
		}
		// Remove old key.
		if oldPublicKey != "" && oldPublicKey != pubKeyStr {
			rmData, _ := json.Marshal(agentmgr.SSHKeyRemoveData{PublicKey: oldPublicKey})
			_ = conn.Send(agentmgr.Message{Type: agentmgr.MsgSSHKeyRemove, Data: rmData})
		}
		pushed++
	}

	reason := req.Reason
	if reason == "" {
		reason = "manual rotation"
	}
	log.Printf("ssh-rotate: rotated hub SSH key to %s (profile %s, reason: %s, pushed to %d agents)", keyType, profileID, reason, pushed) // #nosec G706 -- Logged values are bounded runtime metadata, not free-form user text.

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"status":         "rotated",
		"key_type":       keyType,
		"agents_updated": pushed,
		"agents_total":   len(connectedAssets),
		"public_key":     pubKeyStr,
	})
}
