package resources

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
)

const remoteBookmarkAPIPrefix = "/api/v1/remote-bookmarks"

const (
	remoteBookmarkOwnerType       = "remote_bookmark"
	remoteBookmarkOwnerTypeKey    = "labtether.owner_type"
	remoteBookmarkOwnerIDKey      = "labtether.owner_id"
	remoteBookmarkCreatedByKey    = "labtether.created_by"
	remoteBookmarkMaxLabelLength  = 120
	remoteBookmarkMaxUserLength   = 256
	remoteBookmarkMaxSecretLength = 16 * 1024
	remoteBookmarkProfileNameMax  = 120
)

type remoteBookmarkResponse struct {
	ID             string    `json:"id"`
	Label          string    `json:"label"`
	Protocol       string    `json:"protocol"`
	Host           string    `json:"host"`
	Port           int       `json:"port"`
	HasCredentials bool      `json:"has_credentials"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func redactRemoteBookmark(bookmark persistence.RemoteBookmark) remoteBookmarkResponse {
	return remoteBookmarkResponse{
		ID:             bookmark.ID,
		Label:          bookmark.Label,
		Protocol:       bookmark.Protocol,
		Host:           bookmark.Host,
		Port:           bookmark.Port,
		HasCredentials: bookmark.CredentialID != nil && strings.TrimSpace(*bookmark.CredentialID) != "",
		CreatedAt:      bookmark.CreatedAt,
		UpdatedAt:      bookmark.UpdatedAt,
	}
}

func redactRemoteBookmarks(bookmarks []persistence.RemoteBookmark) []remoteBookmarkResponse {
	out := make([]remoteBookmarkResponse, 0, len(bookmarks))
	for _, bookmark := range bookmarks {
		out = append(out, redactRemoteBookmark(bookmark))
	}
	return out
}

func remoteBookmarkCredentialKind(protocol string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "rdp":
		return credentials.KindRDPPassword, true
	case "vnc", "ard", "spice":
		// SPICE bookmarks use the generic desktop-password profile until the
		// credential inventory exposes a dedicated SPICE kind.
		return credentials.KindVNCPassword, true
	default:
		return "", false
	}
}

func validateRemoteBookmarkFields(label, protocol, host string, port int) (string, string, string, int, error) {
	label = strings.TrimSpace(label)
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if label == "" {
		return "", "", "", 0, errors.New("label is required")
	}
	if len(label) > remoteBookmarkMaxLabelLength {
		return "", "", "", 0, fmt.Errorf("label too long (max %d characters)", remoteBookmarkMaxLabelLength)
	}
	if _, ok := remoteBookmarkCredentialKind(protocol); !ok {
		return "", "", "", 0, errors.New("protocol must be one of: vnc, rdp, spice, ard")
	}
	canonicalHost, canonicalPort, err := securityruntime.ValidateOutboundEndpoint(host, port)
	if err != nil {
		return "", "", "", 0, fmt.Errorf("invalid remote bookmark target: %w", err)
	}
	return label, protocol, canonicalHost, canonicalPort, nil
}

func (d *Deps) appendRemoteBookmarkCredentialAudit(r *http.Request, bookmarkID, action, protocol, decision, reason string) {
	if d.AppendAuditEventBestEffort == nil {
		return
	}
	event := audit.NewEvent("remote_bookmark.credential." + action)
	if d.PrincipalActorID != nil {
		event.ActorID = d.PrincipalActorID(r.Context())
	}
	event.Target = strings.TrimSpace(bookmarkID)
	event.Decision = decision
	event.Reason = strings.TrimSpace(reason)
	event.Details = map[string]any{
		"resource_type": "remote_bookmark",
		"action":        action,
		"protocol":      strings.ToLower(strings.TrimSpace(protocol)),
	}
	d.AppendAuditEventBestEffort(event, "api warning: failed to append remote bookmark credential audit event")
}

func isOwnedRemoteBookmarkCredential(profile credentials.Profile, bookmarkID string) bool {
	return strings.TrimSpace(profile.Metadata[remoteBookmarkOwnerTypeKey]) == remoteBookmarkOwnerType &&
		strings.TrimSpace(profile.Metadata[remoteBookmarkOwnerIDKey]) == strings.TrimSpace(bookmarkID)
}

func (d *Deps) loadRemoteBookmarkCredential(profileID, protocol, bookmarkID string) (credentials.Profile, error) {
	if d.CredentialStore == nil {
		return credentials.Profile{}, errors.New("credential store unavailable")
	}
	profile, ok, err := d.CredentialStore.GetCredentialProfile(strings.TrimSpace(profileID))
	if err != nil {
		return credentials.Profile{}, err
	}
	if !ok {
		return credentials.Profile{}, persistence.ErrNotFound
	}
	wantKind, ok := remoteBookmarkCredentialKind(protocol)
	if !ok || profile.Kind != wantKind {
		return credentials.Profile{}, errors.New("credential kind does not match bookmark protocol")
	}
	if strings.TrimSpace(profile.Status) != "" && !strings.EqualFold(strings.TrimSpace(profile.Status), "active") {
		return credentials.Profile{}, errors.New("credential profile is not active")
	}
	if profile.ExpiresAt != nil && !profile.ExpiresAt.After(time.Now().UTC()) {
		return credentials.Profile{}, errors.New("credential profile has expired")
	}
	if strings.TrimSpace(profile.Metadata[remoteBookmarkOwnerTypeKey]) == remoteBookmarkOwnerType &&
		strings.TrimSpace(profile.Metadata[remoteBookmarkOwnerIDKey]) != strings.TrimSpace(bookmarkID) {
		return credentials.Profile{}, errors.New("credential profile belongs to another remote bookmark")
	}
	return profile, nil
}

func validateRemoteBookmarkInlineCredentials(username, password string) error {
	if len(strings.TrimSpace(username)) > remoteBookmarkMaxUserLength {
		return fmt.Errorf("username too long (max %d characters)", remoteBookmarkMaxUserLength)
	}
	if len(password) > remoteBookmarkMaxSecretLength {
		return fmt.Errorf("password too long (max %d characters)", remoteBookmarkMaxSecretLength)
	}
	return nil
}

func remoteBookmarkCredentialProfileName(label string) string {
	const prefix = "Remote bookmark: "
	label = strings.TrimSpace(label)
	maxLabelBytes := remoteBookmarkProfileNameMax - len(prefix)
	for len(label) > maxLabelBytes {
		_, size := utf8.DecodeLastRuneInString(label)
		if size <= 0 {
			break
		}
		label = label[:len(label)-size]
	}
	return prefix + label
}

func (d *Deps) createOwnedRemoteBookmarkCredential(r *http.Request, bookmark persistence.RemoteBookmark, username, password string) (credentials.Profile, error) {
	if d.CredentialStore == nil || d.SecretsManager == nil {
		return credentials.Profile{}, errors.New("credential encryption unavailable")
	}
	if err := validateRemoteBookmarkInlineCredentials(username, password); err != nil {
		return credentials.Profile{}, err
	}
	username = strings.TrimSpace(username)
	kind, ok := remoteBookmarkCredentialKind(bookmark.Protocol)
	if !ok {
		return credentials.Profile{}, errors.New("unsupported bookmark protocol")
	}
	profileID := idgen.New("cred")
	ciphertext, err := d.SecretsManager.EncryptString(password, profileID)
	if err != nil {
		return credentials.Profile{}, err
	}
	actorID := "system"
	if d.PrincipalActorID != nil {
		actorID = d.PrincipalActorID(r.Context())
	}
	return d.CredentialStore.CreateCredentialProfile(credentials.Profile{
		ID:               profileID,
		Name:             remoteBookmarkCredentialProfileName(bookmark.Label),
		Kind:             kind,
		Username:         username,
		Description:      "Credential managed by a LabTether remote bookmark",
		Status:           "active",
		SecretCiphertext: ciphertext,
		Metadata: map[string]string{
			remoteBookmarkOwnerTypeKey: remoteBookmarkOwnerType,
			remoteBookmarkOwnerIDKey:   bookmark.ID,
			remoteBookmarkCreatedByKey: strings.TrimSpace(actorID),
		},
	})
}

func (d *Deps) deleteOwnedRemoteBookmarkCredential(bookmarkID string, credentialID *string) {
	if d.CredentialStore == nil || credentialID == nil || strings.TrimSpace(*credentialID) == "" {
		return
	}
	profile, ok, err := d.CredentialStore.GetCredentialProfile(strings.TrimSpace(*credentialID))
	if err != nil || !ok || !isOwnedRemoteBookmarkCredential(profile, bookmarkID) {
		return
	}
	_ = d.CredentialStore.DeleteCredentialProfile(profile.ID)
}

// HandleRemoteBookmarks dispatches /api/v1/remote-bookmarks requests.
func (d *Deps) HandleRemoteBookmarks(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, remoteBookmarkAPIPrefix)
	path = strings.TrimPrefix(path, "/")

	if d.RemoteBookmarkStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "remote bookmark store unavailable")
		return
	}
	if d.EnforceRateLimit != nil {
		bucket, limit := "remote_bookmark.read", 300
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			bucket, limit = "remote_bookmark.mutate", 120
		} else if strings.HasSuffix(path, "/credentials") {
			bucket, limit = "remote_bookmark.credential_reveal", 60
		}
		if !d.EnforceRateLimit(w, r, bucket, limit, time.Minute) {
			return
		}
	}

	// Collection routes: /api/v1/remote-bookmarks
	if path == "" {
		switch r.Method {
		case http.MethodGet:
			d.handleListRemoteBookmarks(w, r)
		case http.MethodPost:
			d.handleCreateRemoteBookmark(w, r)
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// Sub-resource routes: /api/v1/remote-bookmarks/{id}[/credentials]
	parts := strings.SplitN(path, "/", 2)
	bmID := strings.TrimSpace(parts[0])
	if bmID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	if len(parts) == 2 {
		action := parts[1]
		if action == "credentials" {
			if r.Method != http.MethodGet {
				servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			d.handleGetRemoteBookmarkCredentials(w, r, bmID)
			return
		}
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPut:
			d.handleUpdateRemoteBookmark(w, r, bmID)
		case http.MethodDelete:
			d.handleDeleteRemoteBookmark(w, r, bmID)
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	servicehttp.WriteError(w, http.StatusNotFound, "not found")
}

// --- List ---

func (d *Deps) handleListRemoteBookmarks(w http.ResponseWriter, r *http.Request) {
	bookmarks, err := d.RemoteBookmarkStore.ListRemoteBookmarks(r.Context())
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list remote bookmarks")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, redactRemoteBookmarks(bookmarks))
}

// --- Create ---

type remoteBookmarkCreateRequest struct {
	Label        string  `json:"label"`
	Protocol     string  `json:"protocol"`
	Host         string  `json:"host"`
	Port         int     `json:"port"`
	CredentialID *string `json:"credential_id,omitempty"`
	Username     *string `json:"username,omitempty"`
	Password     *string `json:"password,omitempty"`
}

func (d *Deps) handleCreateRemoteBookmark(w http.ResponseWriter, r *http.Request) {
	var req remoteBookmarkCreateRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		return
	}
	label, protocol, host, port, err := validateRemoteBookmarkFields(req.Label, req.Protocol, req.Host, req.Port)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	hasInlineCredentials := req.Username != nil || req.Password != nil
	if hasInlineCredentials && req.CredentialID != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "credential_id cannot be combined with username or password")
		return
	}
	if (hasInlineCredentials || req.CredentialID != nil) && !apiv2.RequireScope(w, r, "credentials:use") {
		d.appendRemoteBookmarkCredentialAudit(r, "", "saved", protocol, "denied", "insufficient_scope")
		return
	}
	if req.CredentialID != nil && strings.TrimSpace(*req.CredentialID) != "" {
		if _, err := d.loadRemoteBookmarkCredential(*req.CredentialID, protocol, ""); err != nil {
			d.appendRemoteBookmarkCredentialAudit(r, "", "saved", protocol, "denied", "credential_unavailable")
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid credential profile for bookmark")
			return
		}
	}
	if hasInlineCredentials {
		username, password := "", ""
		if req.Username != nil {
			username = *req.Username
		}
		if req.Password != nil {
			password = *req.Password
		}
		if err := validateRemoteBookmarkInlineCredentials(username, password); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	bm := &persistence.RemoteBookmark{
		Label:    label,
		Protocol: protocol,
		Host:     host,
		Port:     port,
	}
	if !hasInlineCredentials && req.CredentialID != nil && strings.TrimSpace(*req.CredentialID) != "" {
		credentialID := strings.TrimSpace(*req.CredentialID)
		bm.CredentialID = &credentialID
	}
	if err := d.RemoteBookmarkStore.CreateRemoteBookmark(r.Context(), bm); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create remote bookmark")
		return
	}
	if hasInlineCredentials {
		username, password := "", ""
		if req.Username != nil {
			username = *req.Username
		}
		if req.Password != nil {
			password = *req.Password
		}
		profile, createErr := d.createOwnedRemoteBookmarkCredential(r, *bm, username, password)
		if createErr != nil {
			_ = d.RemoteBookmarkStore.DeleteRemoteBookmark(r.Context(), bm.ID)
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to secure bookmark credentials")
			return
		}
		bm.CredentialID = &profile.ID
		if updateErr := d.RemoteBookmarkStore.UpdateRemoteBookmark(r.Context(), *bm); updateErr != nil {
			_ = d.CredentialStore.DeleteCredentialProfile(profile.ID)
			_ = d.RemoteBookmarkStore.DeleteRemoteBookmark(r.Context(), bm.ID)
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to link bookmark credentials")
			return
		}
		d.appendRemoteBookmarkCredentialAudit(r, bm.ID, "saved", bm.Protocol, "applied", "")
	}

	servicehttp.WriteJSON(w, http.StatusCreated, redactRemoteBookmark(*bm))
}

// --- Update ---

type remoteBookmarkUpdateRequest struct {
	Label        string  `json:"label"`
	Protocol     string  `json:"protocol"`
	Host         string  `json:"host"`
	Port         int     `json:"port"`
	CredentialID *string `json:"credential_id,omitempty"`
	Username     *string `json:"username,omitempty"`
	Password     *string `json:"password,omitempty"`
}

func (d *Deps) handleUpdateRemoteBookmark(w http.ResponseWriter, r *http.Request, bmID string) {
	existing, err := d.RemoteBookmarkStore.GetRemoteBookmark(r.Context(), bmID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "remote bookmark not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load remote bookmark")
		return
	}

	var req remoteBookmarkUpdateRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		return
	}
	label, protocol, host, port := existing.Label, existing.Protocol, existing.Host, existing.Port
	if strings.TrimSpace(req.Label) != "" {
		label = req.Label
	}
	if strings.TrimSpace(req.Protocol) != "" {
		protocol = req.Protocol
	}
	if strings.TrimSpace(req.Host) != "" {
		host = req.Host
	}
	if req.Port != 0 {
		port = req.Port
	}
	label, protocol, host, port, err = validateRemoteBookmarkFields(label, protocol, host, port)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	hasInlineCredentials := req.Username != nil || req.Password != nil
	if hasInlineCredentials && req.CredentialID != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "credential_id cannot be combined with username or password")
		return
	}
	credentialsChanged := hasInlineCredentials || req.CredentialID != nil
	protocolChangedWithCredential := protocol != existing.Protocol && existing.CredentialID != nil
	if (credentialsChanged || protocolChangedWithCredential) && !apiv2.RequireScope(w, r, "credentials:use") {
		d.appendRemoteBookmarkCredentialAudit(r, existing.ID, "saved", protocol, "denied", "insufficient_scope")
		return
	}
	if hasInlineCredentials {
		username, password := "", ""
		if req.Username != nil {
			username = *req.Username
		}
		if req.Password != nil {
			password = *req.Password
		}
		if err := validateRemoteBookmarkInlineCredentials(username, password); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	oldCredentialID := existing.CredentialID
	var createdProfile *credentials.Profile
	newCredentialID := oldCredentialID
	if hasInlineCredentials {
		username, password := "", ""
		if req.Username != nil {
			username = *req.Username
		}
		if req.Password != nil {
			password = *req.Password
		}
		candidate := *existing
		candidate.Label, candidate.Protocol, candidate.Host, candidate.Port = label, protocol, host, port
		profile, createErr := d.createOwnedRemoteBookmarkCredential(r, candidate, username, password)
		if createErr != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to secure bookmark credentials")
			return
		}
		createdProfile = &profile
		newCredentialID = &profile.ID
	} else if req.CredentialID != nil {
		credentialID := strings.TrimSpace(*req.CredentialID)
		if credentialID == "" {
			newCredentialID = nil
		} else {
			if _, loadErr := d.loadRemoteBookmarkCredential(credentialID, protocol, existing.ID); loadErr != nil {
				d.appendRemoteBookmarkCredentialAudit(r, existing.ID, "saved", protocol, "denied", "credential_unavailable")
				servicehttp.WriteError(w, http.StatusBadRequest, "invalid credential profile for bookmark")
				return
			}
			newCredentialID = &credentialID
		}
	} else if protocolChangedWithCredential {
		if _, loadErr := d.loadRemoteBookmarkCredential(*existing.CredentialID, protocol, existing.ID); loadErr != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "bookmark credentials must be replaced when changing protocol")
			return
		}
	}

	existing.Label, existing.Protocol, existing.Host, existing.Port = label, protocol, host, port
	existing.CredentialID = newCredentialID

	if err := d.RemoteBookmarkStore.UpdateRemoteBookmark(r.Context(), *existing); err != nil {
		if createdProfile != nil {
			_ = d.CredentialStore.DeleteCredentialProfile(createdProfile.ID)
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update remote bookmark")
		return
	}
	if credentialsChanged {
		oldID, newID := "", ""
		if oldCredentialID != nil {
			oldID = strings.TrimSpace(*oldCredentialID)
		}
		if newCredentialID != nil {
			newID = strings.TrimSpace(*newCredentialID)
		}
		if oldID != newID {
			d.deleteOwnedRemoteBookmarkCredential(existing.ID, oldCredentialID)
		}
		d.appendRemoteBookmarkCredentialAudit(r, existing.ID, "saved", existing.Protocol, "applied", "")
	}

	servicehttp.WriteJSON(w, http.StatusOK, redactRemoteBookmark(*existing))
}

// --- Delete ---

func (d *Deps) handleDeleteRemoteBookmark(w http.ResponseWriter, r *http.Request, bmID string) {
	existing, err := d.RemoteBookmarkStore.GetRemoteBookmark(r.Context(), bmID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "remote bookmark not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load remote bookmark")
		return
	}
	if existing.CredentialID != nil && !apiv2.RequireScope(w, r, "credentials:use") {
		d.appendRemoteBookmarkCredentialAudit(r, existing.ID, "deleted", existing.Protocol, "denied", "insufficient_scope")
		return
	}
	err = d.RemoteBookmarkStore.DeleteRemoteBookmark(r.Context(), bmID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "remote bookmark not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete remote bookmark")
		return
	}
	d.deleteOwnedRemoteBookmarkCredential(existing.ID, existing.CredentialID)
	if existing.CredentialID != nil {
		d.appendRemoteBookmarkCredentialAudit(r, existing.ID, "deleted", existing.Protocol, "applied", "")
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Credentials ---

func (d *Deps) handleGetRemoteBookmarkCredentials(w http.ResponseWriter, r *http.Request, bmID string) {
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("Pragma", "no-cache")
	if !apiv2.RequireScope(w, r, "credentials:use") {
		d.appendRemoteBookmarkCredentialAudit(r, bmID, "revealed", "", "denied", "insufficient_scope")
		return
	}
	bookmark, err := d.RemoteBookmarkStore.GetRemoteBookmark(r.Context(), bmID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "remote bookmark not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load remote bookmark")
		return
	}
	if bookmark.CredentialID == nil || strings.TrimSpace(*bookmark.CredentialID) == "" {
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"id": bmID, "username": nil, "password": nil})
		return
	}
	if d.SecretsManager == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential decryption unavailable")
		return
	}
	profile, err := d.loadRemoteBookmarkCredential(*bookmark.CredentialID, bookmark.Protocol, bookmark.ID)
	if err != nil {
		d.appendRemoteBookmarkCredentialAudit(r, bmID, "revealed", bookmark.Protocol, "denied", "credential_unavailable")
		servicehttp.WriteError(w, http.StatusConflict, "bookmark credentials are unavailable")
		return
	}
	password, err := d.SecretsManager.DecryptString(profile.SecretCiphertext, profile.ID)
	if err != nil {
		d.appendRemoteBookmarkCredentialAudit(r, bmID, "revealed", bookmark.Protocol, "denied", "decrypt_failed")
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to decrypt bookmark credentials")
		return
	}
	_ = d.CredentialStore.MarkCredentialProfileUsed(profile.ID, time.Now().UTC())
	d.appendRemoteBookmarkCredentialAudit(r, bmID, "revealed", bookmark.Protocol, "applied", "")
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"id":       bmID,
		"username": profile.Username,
		"password": password,
	})
}
