package terminal

import (
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

const (
	hostKeyTrustedAuditType = "terminal.ssh_host_key.trusted"
	hostKeyRemovedAuditType = "terminal.ssh_host_key.removed"
)

// hostKeyRequest is the body of POST /terminal/host-keys/{check,trust,forget}.
// With bookmark_id the host and port come from that bookmark and its pin is
// used; otherwise host and port name a Quick connect target.
type hostKeyRequest struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	BookmarkID string `json:"bookmark_id,omitempty"`
	// HostKey (trust) is the exact key a check showed the person.
	HostKey string `json:"host_key,omitempty"`
	// StrictHostKey (check) false asks for an unchecked connection.
	StrictHostKey *bool `json:"strict_host_key,omitempty"`
}

// HandleSSHHostKeys serves trust on first use for terminal SSH: check reads
// the key a host shows (stopping before any login), trust pins the key the
// person was shown, forget removes a pin. Each needs the same rights as
// starting a terminal session on that host.
func (d *Deps) HandleSSHHostKeys(w http.ResponseWriter, r *http.Request) {
	action := strings.TrimPrefix(r.URL.Path, "/terminal/host-keys/")
	if action != "check" && action != "trust" && action != "forget" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.EnforceRateLimit(w, r, "terminal.host_key."+action, 60, time.Minute) {
		return
	}
	var req hostKeyRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteJSONBodyError(w, err)
		return
	}
	host, port, bookmark, ok := d.hostKeyTarget(w, r, req)
	if !ok {
		return
	}
	switch action {
	case "check":
		d.checkSSHHostKey(w, r, req, host, port, bookmark)
	case "trust":
		d.trustSSHHostKey(w, r, req, host, port, bookmark)
	default:
		d.forgetSSHHostKey(w, r, host, port, bookmark)
	}
}

// hostKeyTarget resolves what a host key request is about and applies the
// checks for starting a terminal session there.
func (d *Deps) hostKeyTarget(w http.ResponseWriter, r *http.Request, req hostKeyRequest) (string, int, *terminal.Bookmark, bool) {
	var bookmark *terminal.Bookmark
	host, port := strings.TrimSpace(req.Host), req.Port
	if id := strings.TrimSpace(req.BookmarkID); id != "" {
		if d.TerminalBookmarkStore == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "terminal bookmarks are unavailable")
			return "", 0, nil, false
		}
		found, exists, err := d.TerminalBookmarkStore.GetBookmark(id)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load bookmark")
			return "", 0, nil, false
		}
		if !exists || !d.canAccessOwnedSession(r, found.ActorID) {
			servicehttp.WriteError(w, http.StatusNotFound, "This bookmark no longer exists.")
			return "", 0, nil, false
		}
		var isHost bool
		if host, port, isHost = bookmarkHostTarget(found); !isHost {
			servicehttp.WriteError(w, http.StatusBadRequest, "This bookmark opens a device. Its SSH host key is set in the device's Connect panel.")
			return "", 0, nil, false
		}
		bookmark = &found
	}
	if err := ValidateQuickConnectHost(host); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return "", 0, nil, false
	}
	if port == 0 {
		port = 22
	}
	if port < 1 || port > 65535 {
		servicehttp.WriteError(w, http.StatusBadRequest, "port must be between 1 and 65535")
		return "", 0, nil, false
	}
	if !d.authorizeTerminalStart(w, r, host) {
		return "", 0, nil, false
	}
	return host, port, bookmark, true
}

func (d *Deps) savedHostKey(host string, port int, bookmark *terminal.Bookmark) (string, error) {
	if bookmark != nil {
		return bookmark.PinnedHostKey(), nil
	}
	return d.knownHostKey(host, port)
}

func (d *Deps) checkSSHHostKey(w http.ResponseWriter, r *http.Request, req hostKeyRequest, host string, port int, bookmark *terminal.Bookmark) {
	if req.StrictHostKey != nil && !*req.StrictHostKey && shared.InsecureSSHHostKeysAllowed() {
		servicehttp.WriteJSON(w, http.StatusOK, hostKeyCheck{Status: hostKeyUnchecked, Host: host, Port: port})
		return
	}
	saved, err := d.savedHostKey(host, port, bookmark)
	if err != nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "LabTether could not read its saved SSH host keys.")
		return
	}
	result, err := d.checkHostKey(r.Context(), host, port, saved)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, hostKeyReadFailedMessage(host, port, err))
		return
	}
	if bookmark != nil {
		result.BookmarkID = bookmark.ID
	}
	d.checkedKeys().remember(d.hostKeyCheckID(r, host, port, bookmark), result.HostKey, time.Now())
	servicehttp.WriteJSON(w, http.StatusOK, result)
}

// hostKeyCheckID names one person's check of one host, port and bookmark.
func (d *Deps) hostKeyCheckID(r *http.Request, host string, port int, bookmark *terminal.Bookmark) string {
	bookmarkID := ""
	if bookmark != nil {
		bookmarkID = bookmark.ID
	}
	return checkedHostKeyID(d.PrincipalActorID(r.Context()), terminal.SSHPinTarget(host, port), bookmarkID)
}

func (d *Deps) trustSSHHostKey(w http.ResponseWriter, r *http.Request, req hostKeyRequest, host string, port int, bookmark *terminal.Bookmark) {
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(req.HostKey)))
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "host_key must be the SSH public key the check showed")
		return
	}
	authorized := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
	fingerprint := ssh.FingerprintSHA256(key)
	trusted := hostKeyCheck{Status: hostKeyTrusted, Host: host, Port: port, Fingerprint: fingerprint}
	stored, err := d.savedHostKey(host, port, bookmark)
	if err != nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "LabTether could not read its saved SSH host keys.")
		return
	}
	switch {
	case stored != "" && sameHostKey(stored, key):
		servicehttp.WriteJSON(w, http.StatusOK, trusted)
		return
	case stored != "":
		// A saved key is replaced only after the person removes it.
	case knownHostsListsOther(host, port, key):
		servicehttp.WriteError(w, http.StatusConflict, hostKeyKnownHostsMessage(host, port))
		return
	case !d.checkedKeys().take(d.hostKeyCheckID(r, host, port, bookmark), key, time.Now()):
		// Only a key the host showed to this person's check just now.
		servicehttp.WriteError(w, http.StatusConflict, hostKeyCheckAgainMessage)
		return
	case bookmark != nil:
		keys, ok := d.TerminalBookmarkStore.(persistence.TerminalBookmarkHostKeyStore)
		if !ok {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "LabTether cannot save host keys for bookmarks here.")
			return
		}
		if err := keys.SetBookmarkHostKey(bookmark.ID, authorized, terminal.SSHPinTarget(host, port)); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "LabTether could not save the host key.")
			return
		}
		stored = authorized
	default:
		if d.SSHKnownHostStore == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "LabTether cannot save host keys here.")
			return
		}
		known, err := d.SSHKnownHostStore.AddSSHKnownHost(terminal.SSHKnownHost{
			Host: host, Port: port, HostKey: authorized, Fingerprint: fingerprint, PinnedBy: d.PrincipalActorID(r.Context()),
		})
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "LabTether could not save the host key.")
			return
		}
		stored = known.HostKey
	}
	if !sameHostKey(stored, key) {
		servicehttp.WriteJSON(w, http.StatusConflict, map[string]any{
			"error":             hostKeyAlreadySavedMessage(host, port),
			"saved_fingerprint": hostKeyFingerprint(stored),
		})
		return
	}
	d.auditHostKey(r, hostKeyTrustedAuditType, host, port, fingerprint, bookmark)
	servicehttp.WriteJSON(w, http.StatusOK, trusted)
}

func (d *Deps) forgetSSHHostKey(w http.ResponseWriter, r *http.Request, host string, port int, bookmark *terminal.Bookmark) {
	removed := ""
	if bookmark != nil {
		keys, ok := d.TerminalBookmarkStore.(persistence.TerminalBookmarkHostKeyStore)
		if !ok {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "LabTether cannot save host keys for bookmarks here.")
			return
		}
		if removed = strings.TrimSpace(bookmark.HostKey); removed != "" {
			if err := keys.SetBookmarkHostKey(bookmark.ID, "", ""); err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "LabTether could not remove the host key.")
				return
			}
		}
	} else if d.SSHKnownHostStore != nil {
		known, found, err := d.SSHKnownHostStore.GetSSHKnownHost(host, port)
		if err == nil && found {
			_, err = d.SSHKnownHostStore.DeleteSSHKnownHost(host, port)
		}
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "LabTether could not remove the host key.")
			return
		}
		if found {
			removed = known.HostKey
		}
	}
	if removed != "" {
		d.auditHostKey(r, hostKeyRemovedAuditType, host, port, hostKeyFingerprint(removed), bookmark)
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"removed": removed != "", "host": host, "port": port})
}

// auditHostKey writes the named row for trusting or removing a host key. The
// key itself is public; the row keeps its fingerprint.
func (d *Deps) auditHostKey(r *http.Request, eventType, host string, port int, fingerprint string, bookmark *terminal.Bookmark) {
	if d.AppendAuditEventBestEffort == nil {
		return
	}
	event := audit.NewEvent(eventType)
	event.ActorID = d.PrincipalActorID(r.Context())
	event.Target = terminal.SSHPinTarget(host, port)
	event.TargetName = event.Target
	event.Decision = "applied"
	details := map[string]any{"host": host, "port": port, "fingerprint": fingerprint}
	if bookmark != nil {
		details["bookmark_id"] = bookmark.ID
		details["bookmark"] = bookmark.Title
	}
	event.Details = details
	d.AppendAuditEventBestEffort(event, "api warning: failed to append ssh host key audit event")
}
