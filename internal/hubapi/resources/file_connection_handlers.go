package resources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/fileproto"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
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
		Name:         req.Name,
		Protocol:     req.Protocol,
		Host:         req.Host,
		Port:         req.Port,
		InitialPath:  req.InitialPath,
		CredentialID: &created.ID,
		ExtraConfig:  req.ExtraConfig,
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

	existing, err := d.FileConnectionStore.GetFileConnection(r.Context(), connID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "file connection not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load file connection")
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
		existing.ExtraConfig = req.ExtraConfig
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
		if _, err := d.CredentialStore.UpdateCredentialProfile(profile); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update credential profile")
			return
		}
	}

	if !hasProfile {
		if err := validateFileConnectionFields(existing.Name, existing.Protocol, existing.Host, effectiveUsername, secret, passphrase, effectiveAuthMethod, true, true); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		created, err := d.createFileConnectionCredentialProfile(existing.Name, existing.Protocol, effectiveUsername, desiredKind, secret, passphrase)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create credential profile")
			return
		}
		existing.CredentialID = &created.ID
	}

	if err := d.FileConnectionStore.UpdateFileConnection(r.Context(), existing); err != nil {
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

	// Remove any cached pool session.
	if d.FileProtoPool != nil {
		d.FileProtoPool.Remove(connID)
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": connID})
}

// --- Test (stateless) ---

type fileConnectionTestRequest struct {
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

func (d *Deps) handleFileConnectionTestStateless(w http.ResponseWriter, r *http.Request) {
	var req fileConnectionTestRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		return
	}
	req.Protocol = strings.TrimSpace(req.Protocol)
	req.Host = strings.TrimSpace(req.Host)
	req.Username = strings.TrimSpace(req.Username)
	req.AuthMethod = strings.TrimSpace(req.AuthMethod)
	req.InitialPath = strings.TrimSpace(req.InitialPath)

	if req.Protocol == "" || req.Host == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "protocol and host are required")
		return
	}

	port := 0
	if req.Port != nil {
		port = *req.Port
	}
	if port == 0 {
		port = fileproto.DefaultPort(req.Protocol)
	}

	initialPath := req.InitialPath
	if initialPath == "" {
		initialPath = "/"
	}

	config := fileproto.ConnectionConfig{
		Protocol:    req.Protocol,
		Host:        req.Host,
		Port:        port,
		Username:    req.Username,
		Secret:      strings.TrimSpace(req.Secret),
		Passphrase:  strings.TrimSpace(req.Passphrase),
		AuthMethod:  req.AuthMethod,
		InitialPath: initialPath,
		ExtraConfig: req.ExtraConfig,
	}

	result := d.testFileConnection(r.Context(), config)
	servicehttp.WriteJSON(w, http.StatusOK, result)
}

// --- Test (saved) ---

func (d *Deps) handleFileConnectionTestSaved(w http.ResponseWriter, r *http.Request, connID string) {
	if d.SecretsManager == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential encryption not configured")
		return
	}
	if d.CredentialStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential store unavailable")
		return
	}

	fc, err := d.FileConnectionStore.GetFileConnection(r.Context(), connID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "file connection not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load file connection")
		return
	}

	config, err := d.buildConnectionConfig(fc)
	if err != nil {
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	result := d.testFileConnection(r.Context(), config)
	servicehttp.WriteJSON(w, http.StatusOK, result)
}

// --- helpers ---

func (d *Deps) buildConnectionConfig(fc *persistence.FileConnection) (fileproto.ConnectionConfig, error) {
	port := 0
	if fc.Port != nil {
		port = *fc.Port
	}
	if port == 0 {
		port = fileproto.DefaultPort(fc.Protocol)
	}

	initialPath := fc.InitialPath
	if initialPath == "" {
		initialPath = "/"
	}

	config := fileproto.ConnectionConfig{
		Protocol:    fc.Protocol,
		Host:        fc.Host,
		Port:        port,
		InitialPath: initialPath,
		ExtraConfig: fc.ExtraConfig,
	}

	if fc.CredentialID != nil && *fc.CredentialID != "" {
		profile, ok, err := d.CredentialStore.GetCredentialProfile(*fc.CredentialID)
		if err != nil {
			return config, fmt.Errorf("failed to load credential profile: %w", err)
		}
		if !ok {
			return config, fmt.Errorf("credential profile %s not found", *fc.CredentialID)
		}

		secret, err := d.SecretsManager.DecryptString(profile.SecretCiphertext, profile.ID)
		if err != nil {
			return config, fmt.Errorf("failed to decrypt credentials: %w", err)
		}

		config.Username = profile.Username
		config.Secret = secret

		// Derive auth method from credential kind.
		switch profile.Kind {
		case credentials.KindSSHPrivateKey:
			config.AuthMethod = "private_key"
			// Decrypt passphrase if present.
			if profile.PassphraseCiphertext != "" {
				passphrase, err := d.SecretsManager.DecryptString(profile.PassphraseCiphertext, profile.ID)
				if err != nil {
					return config, fmt.Errorf("failed to decrypt passphrase: %w", err)
				}
				config.Passphrase = passphrase
			}
		default:
			config.AuthMethod = "password"
		}

		// Mark the credential as used.
		_ = d.CredentialStore.MarkCredentialProfileUsed(profile.ID, time.Now().UTC())
	}

	return config, nil
}

func (d *Deps) testFileConnection(ctx context.Context, config fileproto.ConnectionConfig) map[string]any {
	if d.FileProtoPool == nil {
		return map[string]any{"success": false, "error": "file protocol pool not initialized"}
	}

	// Use a temporary connection ID for testing so we don't pollute the pool.
	testID := "test-" + GenerateRequestID()
	start := time.Now()

	fs, err := d.FileProtoPool.Get(ctx, testID, config)
	if err != nil {
		return map[string]any{
			"success": false,
			"error":   fmt.Sprintf("connection failed: %s", err.Error()),
		}
	}
	defer d.FileProtoPool.Remove(testID)

	// Try listing the initial path to verify the connection works end-to-end.
	_, err = fs.List(ctx, config.InitialPath, false)
	latencyMs := time.Since(start).Milliseconds()
	if err != nil {
		return map[string]any{
			"success":    false,
			"error":      fmt.Sprintf("listing failed: %s", err.Error()),
			"latency_ms": latencyMs,
		}
	}

	result := map[string]any{
		"success":    true,
		"latency_ms": latencyMs,
	}

	// For SFTP connections, surface the captured host key fingerprint for TOFU.
	// The frontend can display this to the user and store it in extra_config.
	if sftpClient, ok := fs.(*fileproto.SFTPClient); ok {
		if sftpClient.CapturedHostKey != "" {
			result["host_key"] = sftpClient.CapturedHostKey
			result["fingerprint"] = sftpClient.CapturedFingerprint
		}
	}

	return result
}

func validateFileConnectionRequest(req fileConnectionCreateRequest) error {
	return validateFileConnectionFields(req.Name, req.Protocol, req.Host, req.Username, req.Secret, strings.TrimSpace(req.Passphrase), req.AuthMethod, true, true)
}

func credentialKindForFileProtocol(protocol, authMethod string) string {
	switch protocol {
	case "sftp":
		if authMethod == "private_key" {
			return credentials.KindSSHPrivateKey
		}
		return credentials.KindSSHPassword
	case "ftp":
		return credentials.KindFTPPassword
	case "smb":
		return credentials.KindSMBCredentials
	case "webdav":
		return credentials.KindWebDAVCredentials
	default:
		return ""
	}
}

func authMethodForCredentialKind(kind string) string {
	if kind == credentials.KindSSHPrivateKey {
		return "private_key"
	}
	return "password"
}

func credentialKindUsesPrivateKey(kind string) bool {
	return kind == credentials.KindSSHPrivateKey
}

func validateFileConnectionFields(name, protocol, host, username, secret, passphrase, authMethod string, requireName, requireSecret bool) error {
	if requireName && strings.TrimSpace(name) == "" {
		return errors.New("name is required")
	}
	protocol = strings.TrimSpace(protocol)
	if protocol == "" {
		return errors.New("protocol is required")
	}
	if strings.TrimSpace(host) == "" {
		return errors.New("host is required")
	}
	if strings.TrimSpace(username) == "" {
		return errors.New("username is required")
	}
	if requireSecret && strings.TrimSpace(secret) == "" {
		return errors.New("secret is required")
	}

	switch protocol {
	case "sftp":
		if authMethod == "" {
			authMethod = "password"
		}
		if authMethod != "password" && authMethod != "private_key" {
			return errors.New("auth_method must be password or private_key for sftp")
		}
		if authMethod != "private_key" && strings.TrimSpace(passphrase) != "" {
			return errors.New("passphrase is only supported for private_key authentication")
		}
	case "ftp", "smb", "webdav":
		if authMethod == "" {
			authMethod = "password"
		}
		if authMethod != "password" {
			return fmt.Errorf("auth_method must be password for %s", protocol)
		}
		if strings.TrimSpace(passphrase) != "" {
			return errors.New("passphrase is only supported for private_key authentication")
		}
	default:
		return fmt.Errorf("protocol must be one of: sftp, ftp, smb, webdav")
	}

	return nil
}

func fileConnectionCredentialProfileName(connectionName string) string {
	return fmt.Sprintf("File Connection — %s", strings.TrimSpace(connectionName))
}

func fileConnectionCredentialProfileDescription(protocol string) string {
	return fmt.Sprintf("Auto-created for file connection (%s)", strings.TrimSpace(protocol))
}

func (d *Deps) createFileConnectionCredentialProfile(connectionName, protocol, username, kind, secret, passphrase string) (credentials.Profile, error) {
	profileID := idgen.New("cred")
	secretCiphertext, err := d.SecretsManager.EncryptString(strings.TrimSpace(secret), profileID)
	if err != nil {
		return credentials.Profile{}, err
	}

	passphraseCiphertext := ""
	if strings.TrimSpace(passphrase) != "" {
		passphraseCiphertext, err = d.SecretsManager.EncryptString(strings.TrimSpace(passphrase), profileID)
		if err != nil {
			return credentials.Profile{}, err
		}
	}

	return d.CredentialStore.CreateCredentialProfile(credentials.Profile{
		ID:                   profileID,
		Name:                 fileConnectionCredentialProfileName(connectionName),
		Kind:                 strings.TrimSpace(kind),
		Username:             strings.TrimSpace(username),
		Description:          fileConnectionCredentialProfileDescription(protocol),
		Status:               "active",
		SecretCiphertext:     secretCiphertext,
		PassphraseCiphertext: passphraseCiphertext,
	})
}
