package resources

import (
	"errors"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
	"log"
	"net/http"
	"strings"
	"time"
)

const fileConnectionAPIPrefix = "/api/v1/file-connections"

// HandleFileConnections dispatches /api/v1/file-connections requests.
func (d *Deps) HandleFileConnections(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, fileConnectionAPIPrefix)
	path = strings.TrimPrefix(path, "/")

	if d.FileConnectionStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "file connection store unavailable")
		return
	}

	// POST /api/v1/file-connections/test — stateless test (no ID)
	if path == "test" && r.Method == http.MethodPost {
		d.handleFileConnectionTestStateless(w, r)
		return
	}

	// Collection routes: /api/v1/file-connections
	if path == "" {
		switch r.Method {
		case http.MethodGet:
			d.handleListFileConnections(w, r)
		case http.MethodPost:
			d.handleCreateFileConnection(w, r)
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// Sub-resource routes: /api/v1/file-connections/{id}[/test]
	parts := strings.SplitN(path, "/", 2)
	connID := strings.TrimSpace(parts[0])
	if connID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	if len(parts) == 2 {
		action := parts[1]

		if action == "test" {
			if r.Method != http.MethodPost {
				servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			d.handleFileConnectionTestSaved(w, r, connID)
			return
		}

		// File operation actions (list, download, upload, mkdir, delete, rename, copy).
		if IsFileProtoOp(action) {
			d.dispatchFileProtoOp(w, r, connID, action)
			return
		}

		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPut:
			d.handleUpdateFileConnection(w, r, connID)
		case http.MethodDelete:
			d.handleDeleteFileConnection(w, r, connID)
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	servicehttp.WriteError(w, http.StatusNotFound, "not found")
}

// --- List ---

func (d *Deps) handleListFileConnections(w http.ResponseWriter, r *http.Request) {
	connections, err := d.FileConnectionStore.ListFileConnections(r.Context())
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list file connections")
		return
	}
	connections = d.filterFileConnectionsForActor(r, connections)
	for i := range connections {
		connections[i].ExtraConfig = sanitizeLegacyFileConnectionExtraConfig(connections[i].Protocol, connections[i].ExtraConfig)
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"connections": connections})
}

// --- Create ---

type fileConnectionCreateRequest struct {
	Name        string         `json:"name"`
	Protocol    string         `json:"protocol"`
	Host        string         `json:"host"`
	Port        *int           `json:"port,omitempty"`
	InitialPath string         `json:"initial_path"`
	Username    string         `json:"username"`
	Secret      string         `json:"secret"` // #nosec G117 -- Request payload intentionally carries runtime credential material.
	Passphrase  string         `json:"passphrase,omitempty"`
	AuthMethod  string         `json:"auth_method"`
	ExtraConfig map[string]any `json:"extra_config,omitempty"`
}

func (d *Deps) handleCreateFileConnection(w http.ResponseWriter, r *http.Request) {
	if d.SecretsManager == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential encryption not configured")
		return
	}
	if d.CredentialStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential store unavailable")
		return
	}
	var req fileConnectionCreateRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Protocol = strings.TrimSpace(req.Protocol)
	req.Host = strings.TrimSpace(req.Host)
	req.InitialPath = strings.TrimSpace(req.InitialPath)
	req.Username = strings.TrimSpace(req.Username)
	req.Passphrase = strings.TrimSpace(req.Passphrase)
	req.AuthMethod = strings.TrimSpace(req.AuthMethod)

	if err := validateFileConnectionRequest(req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	normalizedExtra, err := NormalizeFileConnectionExtraConfig(req.Protocol, req.ExtraConfig)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.ExtraConfig = normalizedExtra
	if req.Protocol == "sftp" {
		if _, pinned := req.ExtraConfig["host_key"]; !pinned {
			servicehttp.WriteError(w, http.StatusBadRequest, "test and approve the SFTP server key before saving")
			return
		}
	}

	// Derive credential kind from protocol + auth_method.
	kind := credentialKindForFileProtocol(req.Protocol, req.AuthMethod)
	if kind == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "unsupported protocol/auth_method combination")
		return
	}

	created, err := d.createFileConnectionCredentialProfile(req.Name, req.Protocol, req.Username, kind, req.Secret, req.Passphrase)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create credential profile")
		return
	}

	// Create file connection record.
	fc := &persistence.FileConnection{
		ActorID:      d.currentActorID(r),
		Name:         req.Name,
		Protocol:     req.Protocol,
		Host:         req.Host,
		Port:         req.Port,
		InitialPath:  req.InitialPath,
		CredentialID: &created.ID,
		ExtraConfig:  req.ExtraConfig,
	}
	if d.PrincipalActorID != nil && fc.ActorID == "" {
		_ = d.CredentialStore.DeleteCredentialProfile(created.ID)
		servicehttp.WriteError(w, http.StatusForbidden, "actor is required")
		return
	}
	if err := d.FileConnectionStore.CreateFileConnection(r.Context(), fc); err != nil {
		// Best-effort cleanup of the credential profile.
		_ = d.CredentialStore.DeleteCredentialProfile(created.ID)
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create file connection")
		return
	}

	servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"connection": fc})
}

// --- Update ---

type fileConnectionUpdateRequest struct {
	Name        string         `json:"name"`
	Protocol    string         `json:"protocol"`
	Host        string         `json:"host"`
	Port        *int           `json:"port,omitempty"`
	InitialPath string         `json:"initial_path"`
	Username    string         `json:"username"`
	Secret      string         `json:"secret"` // #nosec G117 -- Request payload intentionally carries runtime credential material.
	Passphrase  string         `json:"passphrase,omitempty"`
	AuthMethod  string         `json:"auth_method"`
	ExtraConfig map[string]any `json:"extra_config,omitempty"`
}

func (d *Deps) handleUpdateFileConnection(w http.ResponseWriter, r *http.Request, connID string) {
	if d.SecretsManager == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential encryption not configured")
		return
	}
	if d.CredentialStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential store unavailable")
		return
	}
	atomicStore, ok := d.FileConnectionStore.(persistence.FileConnectionCredentialStore)
	if !ok {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "atomic file connection updates unavailable")
		return
	}

	existing, err := d.FileConnectionStore.GetFileConnection(r.Context(), connID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "file connection not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load file connection")
		return
	}
	if !d.canAccessFileConnection(r, existing) {
		servicehttp.WriteError(w, http.StatusForbidden, "file connection access denied")
		return
	}

	var req fileConnectionUpdateRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Protocol = strings.TrimSpace(req.Protocol)
	req.Host = strings.TrimSpace(req.Host)
	req.InitialPath = strings.TrimSpace(req.InitialPath)
	req.Username = strings.TrimSpace(req.Username)
	req.Passphrase = strings.TrimSpace(req.Passphrase)
	req.AuthMethod = strings.TrimSpace(req.AuthMethod)

	previousProtocol := existing.Protocol
	previousHost := existing.Host
	previousPort := effectiveFileConnectionPort(existing.Protocol, existing.Port)
	previousExtraConfig := existing.ExtraConfig
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Protocol != "" {
		existing.Protocol = req.Protocol
	}
	if req.Host != "" {
		existing.Host = req.Host
	}
	if req.Port != nil {
		existing.Port = req.Port
	}
	if req.InitialPath != "" {
		existing.InitialPath = req.InitialPath
	}
	if req.ExtraConfig != nil {
		existing.ExtraConfig = preserveExistingSFTPHostKey(existing.Protocol, previousExtraConfig, req.ExtraConfig)
	}
	normalizedExtra, err := NormalizeFileConnectionExtraConfig(existing.Protocol, existing.ExtraConfig)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	existing.ExtraConfig = normalizedExtra
	if existing.Protocol == "sftp" {
		if _, pinned := normalizedExtra["host_key"]; !pinned {
			endpointChanged := previousProtocol != "sftp" ||
				!strings.EqualFold(strings.TrimSpace(previousHost), strings.TrimSpace(existing.Host)) ||
				previousPort != effectiveFileConnectionPort(existing.Protocol, existing.Port)
			if endpointChanged {
				servicehttp.WriteError(w, http.StatusBadRequest, "test and approve the SFTP server key before saving")
				return
			}
		}
	}

	secret := strings.TrimSpace(req.Secret)
	passphrase := strings.TrimSpace(req.Passphrase)

	var profile credentials.Profile
	hasProfile := false
	if existing.CredentialID != nil && strings.TrimSpace(*existing.CredentialID) != "" {
		loaded, ok, err := d.CredentialStore.GetCredentialProfile(*existing.CredentialID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load credential profile")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusInternalServerError, "linked credential profile not found")
			return
		}
		profile = loaded
		hasProfile = true
	}

	effectiveUsername := req.Username
	if effectiveUsername == "" && hasProfile {
		effectiveUsername = strings.TrimSpace(profile.Username)
	}
	effectiveAuthMethod := req.AuthMethod
	if effectiveAuthMethod == "" && hasProfile {
		effectiveAuthMethod = authMethodForCredentialKind(profile.Kind)
	}
	desiredKind := credentialKindForFileProtocol(existing.Protocol, effectiveAuthMethod)
	if desiredKind == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "unsupported protocol/auth_method combination")
		return
	}
	if err := validateFileConnectionFields(existing.Name, existing.Protocol, existing.Host, effectiveUsername, secret, passphrase, effectiveAuthMethod, true, false); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if hasProfile && credentialKindUsesPrivateKey(profile.Kind) != credentialKindUsesPrivateKey(desiredKind) && secret == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "secret is required when changing between password and private_key authentication")
		return
	}
	finishPoolMutation := func() {}
	if d.FileProtoPool != nil {
		finishPoolMutation = d.FileProtoPool.BeginConnectionMutation(connID)
	}
	defer finishPoolMutation()

	if hasProfile {
		profile.Name = fileConnectionCredentialProfileName(existing.Name)
		profile.Kind = desiredKind
		profile.Username = effectiveUsername
		profile.Description = fileConnectionCredentialProfileDescription(existing.Protocol)
		if secret != "" {
			profile.SecretCiphertext, err = d.SecretsManager.EncryptString(secret, profile.ID)
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to encrypt credential secret")
				return
			}
		}
		switch {
		case desiredKind != credentials.KindSSHPrivateKey:
			profile.PassphraseCiphertext = ""
		case req.Passphrase != "":
			profile.PassphraseCiphertext, err = d.SecretsManager.EncryptString(passphrase, profile.ID)
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to encrypt passphrase")
				return
			}
		}
		if secret != "" || req.Passphrase != "" {
			now := time.Now().UTC()
			profile.RotatedAt = &now
		}
	}

	if !hasProfile {
		if err := validateFileConnectionFields(existing.Name, existing.Protocol, existing.Host, effectiveUsername, secret, passphrase, effectiveAuthMethod, true, true); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		created, err := d.prepareFileConnectionCredentialProfile(existing.Name, existing.Protocol, effectiveUsername, desiredKind, secret, passphrase)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create credential profile")
			return
		}
		profile = created
		existing.CredentialID = &created.ID
	}

	if _, err := atomicStore.UpdateFileConnectionWithCredential(r.Context(), existing, profile, !hasProfile); err != nil {
		if errors.Is(err, persistence.ErrFileConnectionChanged) {
			servicehttp.WriteError(w, http.StatusConflict, "file connection changed; reload and try again")
			return
		}
		if errors.Is(err, persistence.ErrNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "file connection not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update file connection")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"connection": existing})
}

// --- Delete ---

func (d *Deps) handleDeleteFileConnection(w http.ResponseWriter, r *http.Request, connID string) {
	existing, err := d.FileConnectionStore.GetFileConnection(r.Context(), connID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "file connection not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load file connection")
		return
	}
	if !d.canAccessFileConnection(r, existing) {
		servicehttp.WriteError(w, http.StatusForbidden, "file connection access denied")
		return
	}
	finishPoolMutation := func() {}
	if d.FileProtoPool != nil {
		finishPoolMutation = d.FileProtoPool.BeginConnectionMutation(connID)
	}
	defer finishPoolMutation()

	// Delete the file connection first.
	if err := d.FileConnectionStore.DeleteFileConnection(r.Context(), connID); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete file connection")
		return
	}

	// Clean up the linked credential profile.
	if existing.CredentialID != nil && d.CredentialStore != nil {
		if err := d.CredentialStore.DeleteCredentialProfile(*existing.CredentialID); err != nil {
			log.Printf("file-connections: failed to delete credential profile %s: %v", *existing.CredentialID, err) // #nosec G706 -- Credential IDs are hub-generated identifiers, not raw user input.
		}
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": connID})
}

func (d *Deps) currentActorID(r *http.Request) string {
	if d.PrincipalActorID == nil || r == nil {
		return ""
	}
	return strings.TrimSpace(d.PrincipalActorID(r.Context()))
}

func (d *Deps) canAccessFileConnection(r *http.Request, fc *persistence.FileConnection) bool {
	if fc == nil {
		return false
	}
	if d.PrincipalActorID == nil {
		return true
	}
	actorID := d.currentActorID(r)
	if actorID == "" {
		return false
	}
	return strings.TrimSpace(fc.ActorID) == actorID
}

func (d *Deps) filterFileConnectionsForActor(r *http.Request, connections []persistence.FileConnection) []persistence.FileConnection {
	if d.PrincipalActorID == nil {
		return connections
	}
	actorID := d.currentActorID(r)
	if actorID == "" || len(connections) == 0 {
		return []persistence.FileConnection{}
	}
	filtered := make([]persistence.FileConnection, 0, len(connections))
	for _, connection := range connections {
		if strings.TrimSpace(connection.ActorID) == actorID {
			filtered = append(filtered, connection)
		}
	}
	return filtered
}
