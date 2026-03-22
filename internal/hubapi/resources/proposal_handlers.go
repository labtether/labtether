package resources

import (
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/servicehttp"
)

// HandleProposals handles GET /discovery/proposals — returns pending proposals.
// Registered at /discovery/proposals (exact match).
func (d *Deps) HandleProposals(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/discovery/proposals" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.EdgeStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "edge store unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		proposals, err := d.EdgeStore.ListProposals()
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list proposals")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"proposals": proposals})

	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleProposalActions handles POST /discovery/proposals/{id}/accept and
// POST /discovery/proposals/{id}/dismiss.
// Registered at /discovery/proposals/ (prefix match).
func (d *Deps) HandleProposalActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/discovery/proposals/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "proposal path not found")
		return
	}
	if d.EdgeStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "edge store unavailable")
		return
	}

	// Expected form: {id}/accept or {id}/dismiss
	parts := strings.SplitN(path, "/", 2)
	proposalID := strings.TrimSpace(parts[0])
	if proposalID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "proposal id is required")
		return
	}

	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "action is required (accept or dismiss)")
		return
	}
	action := strings.TrimSpace(parts[1])

	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	switch action {
	case "accept":
		if err := d.EdgeStore.AcceptProposal(proposalID); err != nil {
			if strings.Contains(err.Error(), "not found") {
				servicehttp.WriteError(w, http.StatusNotFound, "proposal not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to accept proposal")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "accepted"})

	case "dismiss":
		if err := d.EdgeStore.DismissProposal(proposalID); err != nil {
			if strings.Contains(err.Error(), "not found") {
				servicehttp.WriteError(w, http.StatusNotFound, "proposal not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to dismiss proposal")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "dismissed"})

	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown action: must be accept or dismiss")
	}
}
