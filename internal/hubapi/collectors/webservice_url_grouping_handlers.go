package collectors

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

// --- request types ---

type altURLAddRequest struct {
	WebServiceID string `json:"web_service_id"`
	URL          string `json:"url"`
	Source       string `json:"source"`
}

type neverGroupRuleAddRequest struct {
	URLA string `json:"url_a"`
	URLB string `json:"url_b"`
}

type groupingSettingsPatchRequest struct {
	Values map[string]string `json:"values"`
}

// HandleWebServiceAltURLs handles GET/POST/DELETE /api/v1/services/web/alt-urls/[id].
func (d *Deps) HandleWebServiceAltURLs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		webServiceID := strings.TrimSpace(r.URL.Query().Get("web_service_id"))
		if webServiceID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "web_service_id query parameter is required")
			return
		}

		items := make([]persistence.WebServiceAltURL, 0, 4)
		if d.DB != nil {
			persisted, err := d.DB.ListAltURLsByService(r.Context(), webServiceID)
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list alt URLs")
				return
			}
			items = append(items, persisted...)
		}
		items = append(items, d.syntheticAltURLs(webServiceID, items)...)
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"alt_urls": items})

	case http.MethodPost:
		var req altURLAddRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid alt URL payload")
			return
		}
		req.WebServiceID = strings.TrimSpace(req.WebServiceID)
		req.URL = strings.TrimSpace(req.URL)
		req.Source = strings.TrimSpace(req.Source)
		if req.WebServiceID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "web_service_id is required")
			return
		}
		if req.URL == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "url is required")
			return
		}
		if req.Source == "" {
			req.Source = "manual"
		}
		if d.DB == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		if err := d.DB.UpsertAltURL(r.Context(), req.WebServiceID, req.URL, req.Source); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to add alt URL")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})

	case http.MethodDelete:
		// Parse ID from URL path: /api/v1/services/web/alt-urls/{id}
		trimmed := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/services/web/alt-urls/"))
		if trimmed == "" || trimmed == r.URL.Path {
			servicehttp.WriteError(w, http.StatusBadRequest, "alt URL id is required in path")
			return
		}
		id := strings.TrimSpace(strings.Trim(trimmed, "/"))
		if id == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "alt URL id is required in path")
			return
		}
		if d.DB == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		if err := d.DB.DeleteAltURL(r.Context(), id); err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "alt URL not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete alt URL")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) syntheticAltURLs(webServiceID string, persisted []persistence.WebServiceAltURL) []persistence.WebServiceAltURL {
	if d.WebServiceCoordinator == nil {
		return nil
	}

	services, _ := applyWebServiceURLGrouping(d.WebServiceCoordinator.ListAll(), d.ResolveWebServiceURLGroupingConfig())
	seen := make(map[string]struct{}, len(persisted))
	for _, item := range persisted {
		seen[strings.ToLower(strings.TrimSpace(item.URL))] = struct{}{}
	}

	for _, service := range services {
		if !strings.EqualFold(strings.TrimSpace(service.URL), webServiceID) {
			continue
		}
		aliases := splitAliasCSV(service.Metadata["alt_urls"])
		if len(aliases) == 0 {
			return nil
		}

		out := make([]persistence.WebServiceAltURL, 0, len(aliases))
		for _, alias := range aliases {
			key := strings.ToLower(strings.TrimSpace(alias))
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, persistence.WebServiceAltURL{
				ID:           "auto:" + webServiceID + ":" + alias,
				WebServiceID: webServiceID,
				URL:          alias,
				Source:       "auto",
			})
		}
		return out
	}

	return nil
}

// HandleWebServiceNeverGroupRules handles GET/POST/DELETE /api/v1/services/web/never-group-rules.
func (d *Deps) HandleWebServiceNeverGroupRules(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/services/web/never-group-rules" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if d.DB == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		rules, err := d.DB.ListNeverGroupRules(r.Context())
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list never-group rules")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"rules": rules})

	case http.MethodPost:
		var req neverGroupRuleAddRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid never-group rule payload")
			return
		}
		req.URLA = strings.TrimSpace(req.URLA)
		req.URLB = strings.TrimSpace(req.URLB)
		if req.URLA == "" || req.URLB == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "url_a and url_b are required")
			return
		}
		if d.DB == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		if err := d.DB.UpsertNeverGroupRule(r.Context(), req.URLA, req.URLB); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to add never-group rule")
			return
		}
		d.InvalidateWebServiceURLGroupingConfigCache()
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})

	case http.MethodDelete:
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "id query parameter is required")
			return
		}
		if d.DB == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		if err := d.DB.DeleteNeverGroupRule(r.Context(), id); err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "never-group rule not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete never-group rule")
			return
		}
		d.InvalidateWebServiceURLGroupingConfigCache()
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleWebServiceGroupingSettings handles GET/PATCH /api/v1/services/web/grouping-settings.
func (d *Deps) HandleWebServiceGroupingSettings(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/services/web/grouping-settings" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if d.DB == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		settings, err := d.DB.ListURLGroupingSettings(r.Context())
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list grouping settings")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"settings": settings})

	case http.MethodPatch:
		var req groupingSettingsPatchRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid grouping settings payload")
			return
		}
		if len(req.Values) == 0 {
			servicehttp.WriteError(w, http.StatusBadRequest, "values map is required and must not be empty")
			return
		}
		if d.DB == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		for key, value := range req.Values {
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			if key == "" {
				continue
			}
			if err := d.DB.UpsertURLGroupingSetting(r.Context(), key, value); err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update grouping setting")
				return
			}
		}
		d.InvalidateWebServiceURLGroupingConfigCache()
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleWebServiceGroupingSuggestionResponse handles POST /api/v1/services/web/grouping-suggestions/{id}/{action}.
// Action must be "accept" or "deny".
func (d *Deps) HandleWebServiceGroupingSuggestionResponse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Parse /{id}/{action} from the URL path.
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/v1/services/web/grouping-suggestions/")
	parts := strings.SplitN(strings.Trim(trimmed, "/"), "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "expected path: /grouping-suggestions/{id}/{action}")
		return
	}
	suggestionID := strings.TrimSpace(parts[0])
	action := strings.ToLower(strings.TrimSpace(parts[1]))

	if action != "accept" && action != "deny" {
		servicehttp.WriteError(w, http.StatusBadRequest, "action must be 'accept' or 'deny'")
		return
	}

	if d.DB == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	// Find and remove the suggestion.
	d.URLGroupingSuggestionsMu.Lock()
	remaining, found, ok := removeSuggestion(d.URLGroupingSuggestions, suggestionID)
	d.URLGroupingSuggestions = remaining
	d.URLGroupingSuggestionsMu.Unlock()

	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "suggestion not found")
		return
	}

	ctx := r.Context()
	switch action {
	case "accept":
		if err := d.DB.UpsertAltURL(ctx, found.BaseServiceURL, found.SuggestedURL, "suggestion_accepted"); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to persist accepted alt URL")
			return
		}
	case "deny":
		if err := d.DB.UpsertNeverGroupRule(ctx, found.BaseServiceURL, found.SuggestedURL); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to persist never-group rule")
			return
		}
		d.InvalidateWebServiceURLGroupingConfigCache()
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
