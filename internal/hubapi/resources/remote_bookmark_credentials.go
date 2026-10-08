package resources

import (
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

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

func validateRemoteBookmarkSPICESecurity(protocol, rawMode, rawCAPEM string) (string, string, error) {
	mode := strings.ToLower(strings.TrimSpace(rawMode))
	caPEM := strings.TrimSpace(rawCAPEM)
	if protocol != "spice" {
		if mode != "" || caPEM != "" {
			return "", "", errors.New("SPICE security options are only valid for SPICE")
		}
		return "tls", "", nil
	}
	if mode == "" {
		mode = "tls"
	}
	if mode != "tls" && mode != "cleartext" {
		return "", "", errors.New("spice_security_mode must be tls or cleartext")
	}
	if len(caPEM) > remoteBookmarkMaxSPICECAPEM {
		return "", "", fmt.Errorf("spice_ca_pem too long (max %d bytes)", remoteBookmarkMaxSPICECAPEM)
	}
	if mode == "cleartext" {
		if caPEM != "" {
			return "", "", errors.New("spice_ca_pem cannot be used with cleartext SPICE")
		}
		if !securityruntime.InsecureTransportAllowed() {
			return "", "", errors.New("cleartext SPICE requires LABTETHER_ALLOW_INSECURE_TRANSPORT=true")
		}
		return mode, "", nil
	}
	if caPEM != "" {
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM([]byte(caPEM)); !ok {
			return "", "", errors.New("invalid spice_ca_pem certificate bundle")
		}
	}
	return mode, caPEM, nil
}

func validateRemoteBookmarkVNCTransport(protocol string, allowInsecure bool) error {
	if protocol != "vnc" && protocol != "ard" {
		if allowInsecure {
			return errors.New("allow_insecure_vnc is only valid for VNC or ARD")
		}
		return nil
	}
	if !allowInsecure {
		return errors.New("plain VNC requires allow_insecure_vnc=true")
	}
	if !securityruntime.InsecureTransportAllowed() {
		return errors.New("plain VNC requires LABTETHER_ALLOW_INSECURE_TRANSPORT=true")
	}
	return nil
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
