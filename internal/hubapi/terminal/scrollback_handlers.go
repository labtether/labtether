package terminal

import (
	"net/http"

	"github.com/labtether/labtether/internal/servicehttp"
)

// handleScrollback serves the persisted scrollback buffer for a persistent
// terminal session. Called from HandleSessionActions when the sub-path is
// "scrollback".
func (d *Deps) handleScrollback(w http.ResponseWriter, r *http.Request, persistentSessionID string) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.TerminalScrollbackStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "scrollback not available")
		return
	}
	buffer, err := d.TerminalScrollbackStore.GetScrollback(persistentSessionID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to fetch scrollback")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buffer) // #nosec G705 -- Response is binary octet-stream scrollback, not HTML output.
}
