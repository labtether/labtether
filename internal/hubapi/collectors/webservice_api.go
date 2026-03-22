package collectors

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

const (
	webServiceDetailCompact = "compact"
	webServiceDetailFull    = "full"
)

// HandleWebServices handles GET /api/v1/services/web — list all web services.
func (d *Deps) HandleWebServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.WebServiceCoordinator == nil {
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"services": []any{}})
		return
	}

	hostFilter := r.URL.Query().Get("host")
	serviceIDFilter := strings.TrimSpace(r.URL.Query().Get("service_id"))
	includeHidden := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_hidden")), "true")
	standaloneOnly := r.URL.Query().Get("standalone") == "true"
	detailLevel := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("detail")))
	if detailLevel == "" {
		detailLevel = webServiceDetailFull
	}
	includeExpandedFields := detailLevel != webServiceDetailCompact

	var services []agentmgr.DiscoveredWebService
	if hostFilter != "" {
		services = d.WebServiceCoordinator.ListByHost(hostFilter)
	} else {
		services = d.WebServiceCoordinator.ListAll()
	}
	if services == nil {
		services = []agentmgr.DiscoveredWebService{}
	}
	if standaloneOnly {
		filtered := make([]agentmgr.DiscoveredWebService, 0)
		for _, svc := range services {
			if svc.HostAssetID == "" {
				filtered = append(filtered, svc)
			}
		}
		services = filtered
	}
	if serviceIDFilter != "" {
		filtered := make([]agentmgr.DiscoveredWebService, 0, 1)
		for _, svc := range services {
			if strings.TrimSpace(svc.ID) == serviceIDFilter {
				filtered = append(filtered, svc)
				break
			}
		}
		services = filtered
	}
	grouped, suggestions := applyWebServiceURLGrouping(services, d.ResolveWebServiceURLGroupingConfig())
	services = grouped
	d.URLGroupingSuggestionsMu.Lock()
	d.URLGroupingSuggestions = suggestions
	d.URLGroupingSuggestionsMu.Unlock()
	d.WebServiceCoordinator.AttachHealthSummaries(services)

	// Persist auto-detected alt URLs from grouping engine to database only when
	// expanded fields are requested. Compact polling paths can synthesize the
	// same aliases on demand without paying the write cost on every refresh.
	if includeExpandedFields && d.DB != nil {
		for _, svc := range services {
			if raw, ok := svc.Metadata["alt_urls"]; ok && raw != "" {
				for _, altURL := range strings.Split(raw, ",") {
					altURL = strings.TrimSpace(altURL)
					if altURL != "" {
						if err := d.DB.UpsertAltURL(r.Context(), svc.URL, altURL, "auto"); err != nil {
							slog.Warn("failed to persist auto alt URL", "service_url", svc.URL, "alt_url", altURL, "error", err) // #nosec G706 -- Service and alt URLs are normalized connector values, not raw log text injection.
						}
					}
				}
			}
		}
	}

	if !includeHidden {
		visible := make([]agentmgr.DiscoveredWebService, 0, len(services))
		for _, svc := range services {
			if isHiddenWebService(svc) {
				continue
			}
			visible = append(visible, svc)
		}
		services = visible
	}
	if !includeExpandedFields {
		compact := make([]agentmgr.DiscoveredWebService, len(services))
		for index, svc := range services {
			compact[index] = compactDiscoveredService(svc)
		}
		discoveryStats := d.WebServiceCoordinator.DiscoveryStats(hostFilter)
		resp := map[string]any{
			"services":        compact,
			"discovery_stats": discoveryStats,
		}
		if len(suggestions) > 0 {
			resp["suggestions"] = suggestions
		}
		servicehttp.WriteJSON(w, http.StatusOK, resp)
		return
	}

	// Load persisted alt URLs and build enriched response.
	type serviceWithAltURLs struct {
		agentmgr.DiscoveredWebService
		AltURLs []persistence.WebServiceAltURL `json:"alt_urls,omitempty"`
	}
	var allAltURLs []persistence.WebServiceAltURL
	if d.DB != nil {
		var err error
		allAltURLs, err = d.DB.ListAllAltURLs(r.Context())
		if err != nil {
			slog.Warn("failed to load persisted alt URLs", "error", err)
		}
	}
	altURLsByService := make(map[string][]persistence.WebServiceAltURL, len(allAltURLs))
	for _, alt := range allAltURLs {
		altURLsByService[alt.WebServiceID] = append(altURLsByService[alt.WebServiceID], alt)
	}
	enriched := make([]serviceWithAltURLs, len(services))
	for i, svc := range services {
		enriched[i] = serviceWithAltURLs{
			DiscoveredWebService: svc,
			AltURLs:              altURLsByService[svc.URL],
		}
	}

	discoveryStats := d.WebServiceCoordinator.DiscoveryStats(hostFilter)
	resp := map[string]any{
		"services":        enriched,
		"discovery_stats": discoveryStats,
	}
	if len(suggestions) > 0 {
		resp["suggestions"] = suggestions
	}
	servicehttp.WriteJSON(w, http.StatusOK, resp)
}

func compactDiscoveredService(service agentmgr.DiscoveredWebService) agentmgr.DiscoveredWebService {
	compact := service
	if service.Metadata != nil {
		compact.Metadata = cloneWebServiceMetadataMap(service.Metadata)
	}
	if service.Health != nil {
		health := *service.Health
		health.Recent = nil
		compact.Health = &health
	}
	return compact
}

func cloneWebServiceMetadataMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

// HandleWebServiceCategories handles GET /api/v1/services/web/categories.
func (d *Deps) HandleWebServiceCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.WebServiceCoordinator == nil {
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"categories": []string{}})
		return
	}

	cats := d.WebServiceCoordinator.Categories()
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"categories": cats})
}

func isHiddenWebService(svc agentmgr.DiscoveredWebService) bool {
	if svc.Metadata == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(svc.Metadata["hidden"]), "true")
}
