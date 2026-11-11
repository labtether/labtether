package terminal

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

var errBookmarkNoLogin = errors.New(bookmarkNoLoginMessage)

// resolveBookmarkSSH uses the bookmark a saved session was opened from when
// that bookmark points to a host (not a device) at the session's target: its
// user, saved login and the host key its owner trusted, checked strictly. It
// returns nil, nil for any other session.
func (d *Deps) resolveBookmarkSSH(session terminal.Session) (*terminal.SSHConfig, error) {
	bookmark, ok, err := d.sessionHostBookmark(session)
	if err != nil || !ok {
		return nil, err
	}
	host, port, _ := bookmarkHostTarget(bookmark)
	if strings.TrimSpace(bookmark.CredentialProfileID) == "" {
		return nil, errBookmarkNoLogin
	}
	pinned := bookmark.PinnedHostKey()
	if needsTrustedHostKey(pinned) {
		return nil, errHostKeyUnknown{text: hostKeyUnknownMessage(host, port)}
	}
	return d.ResolveAssetTerminalConfig(credentials.AssetTerminalConfig{
		Host:                host,
		Port:                port,
		Username:            bookmark.Username,
		CredentialProfileID: bookmark.CredentialProfileID,
		StrictHostKey:       true,
		HostKey:             pinned,
	})
}

// sessionHostBookmark finds the bookmark to a host a session's saved session
// was opened from: the session owner's own bookmark, pointing at the
// session's target.
func (d *Deps) sessionHostBookmark(session terminal.Session) (terminal.Bookmark, bool, error) {
	persistentID := strings.TrimSpace(session.PersistentSessionID)
	if persistentID == "" || d.TerminalPersistentStore == nil || d.TerminalBookmarkStore == nil {
		return terminal.Bookmark{}, false, nil
	}
	persistent, ok, err := d.TerminalPersistentStore.GetPersistentSession(persistentID)
	if err != nil {
		return terminal.Bookmark{}, false, fmt.Errorf("failed to load saved session: %w", err)
	}
	if !ok || strings.TrimSpace(persistent.BookmarkID) == "" {
		return terminal.Bookmark{}, false, nil
	}
	bookmark, ok, err := d.TerminalBookmarkStore.GetBookmark(persistent.BookmarkID)
	if err != nil {
		return terminal.Bookmark{}, false, fmt.Errorf("failed to load bookmark: %w", err)
	}
	host, port, isHost := bookmarkHostTarget(bookmark)
	if !ok || !isHost || bookmark.ActorID != session.ActorID ||
		terminal.SSHPinTarget(host, port) != sessionPinTarget(session.Target) {
		return terminal.Bookmark{}, false, nil
	}
	return bookmark, true, nil
}

// requireCredentialUseForSession is requireCredentialUseForTarget plus the
// saved login of the bookmark the session was opened from: using it needs
// credentials:use, as connecting the bookmark does.
func (d *Deps) requireCredentialUseForSession(w http.ResponseWriter, r *http.Request, session terminal.Session) bool {
	if !d.requireCredentialUseForTarget(w, r, session.Target) {
		return false
	}
	bookmark, ok, err := d.sessionHostBookmark(session)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to authorize stored credential use")
		return false
	}
	if ok && strings.TrimSpace(bookmark.CredentialProfileID) != "" {
		return apiv2.RequireScope(w, r, "credentials:use")
	}
	return true
}

// sessionPinTarget reads a "host:port" session target (as bookmark connect
// makes it) in pinned form.
func sessionPinTarget(target string) string {
	target = strings.TrimSpace(target)
	cut := strings.LastIndex(target, ":")
	if cut <= 0 {
		return terminal.SSHPinTarget(target, 22)
	}
	var port int
	if _, err := fmt.Sscanf(target[cut+1:], "%d", &port); err != nil || port <= 0 {
		return ""
	}
	return terminal.SSHPinTarget(target[:cut], port)
}
