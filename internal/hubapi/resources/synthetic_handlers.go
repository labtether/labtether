package resources

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/synthetic"
)

func (d *Deps) HandleSyntheticChecks(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/synthetic-checks" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.SyntheticStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "synthetic store unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		enabledOnly := strings.ToLower(r.URL.Query().Get("enabled")) == "true"
		checks, err := d.SyntheticStore.ListSyntheticChecks(parseLimit(r, 50), enabledOnly)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list synthetic checks")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"checks": checks})
	case http.MethodPost:
		if !d.EnforceRateLimit(w, r, "synthetic.create", 120, time.Minute) {
			return
		}
		var req synthetic.CreateCheckRequest
		if err := d.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid synthetic check payload")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		req.Target = strings.TrimSpace(req.Target)
		if req.Name == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		if req.Target == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "target is required")
			return
		}
		if normalized := synthetic.NormalizeCheckType(req.CheckType); normalized == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "check_type must be one of: http, tcp, dns, tls_cert")
			return
		} else {
			req.CheckType = normalized
		}
		check, err := d.SyntheticStore.CreateSyntheticCheck(req)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create synthetic check")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"check": check})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) HandleSyntheticCheckActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/synthetic-checks/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "synthetic check path not found")
		return
	}
	if d.SyntheticStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "synthetic store unavailable")
		return
	}

	parts := strings.Split(path, "/")
	checkID := strings.TrimSpace(parts[0])
	if checkID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "synthetic check path not found")
		return
	}

	// GET /synthetic-checks/{id}/results
	if len(parts) == 2 && parts[1] == "results" {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		results, err := d.SyntheticStore.ListSyntheticResults(checkID, parseLimit(r, 50))
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list synthetic results")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"results": results})
		return
	}

	if len(parts) > 1 {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown synthetic check action")
		return
	}

	// GET/PATCH/DELETE /synthetic-checks/{id}
	switch r.Method {
	case http.MethodGet:
		check, ok, err := d.SyntheticStore.GetSyntheticCheck(checkID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load synthetic check")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "synthetic check not found")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"check": check})
	case http.MethodPatch, http.MethodPut:
		if !d.EnforceRateLimit(w, r, "synthetic.update", 180, time.Minute) {
			return
		}
		var req synthetic.UpdateCheckRequest
		if err := d.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid synthetic check payload")
			return
		}
		if req.Name != nil {
			trimmed := strings.TrimSpace(*req.Name)
			if trimmed == "" {
				servicehttp.WriteError(w, http.StatusBadRequest, "name cannot be empty")
				return
			}
			req.Name = &trimmed
		}
		if req.Target != nil {
			trimmed := strings.TrimSpace(*req.Target)
			if trimmed == "" {
				servicehttp.WriteError(w, http.StatusBadRequest, "target cannot be empty")
				return
			}
			req.Target = &trimmed
		}
		updated, err := d.SyntheticStore.UpdateSyntheticCheck(checkID, req)
		if err != nil {
			if err == synthetic.ErrCheckNotFound {
				servicehttp.WriteError(w, http.StatusNotFound, "synthetic check not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update synthetic check")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"check": updated})
	case http.MethodDelete:
		if err := d.SyntheticStore.DeleteSyntheticCheck(checkID); err != nil {
			if err == synthetic.ErrCheckNotFound {
				servicehttp.WriteError(w, http.StatusNotFound, "synthetic check not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete synthetic check")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
