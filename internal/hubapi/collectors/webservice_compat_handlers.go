package collectors

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/servicehttp"
)

type webServiceCompatibleAPI struct {
	HostAssetID string  `json:"host_asset_id"`
	ServiceID   string  `json:"service_id"`
	ServiceName string  `json:"service_name"`
	ServiceURL  string  `json:"service_url"`
	Category    string  `json:"category"`
	Source      string  `json:"source"`
	ConnectorID string  `json:"connector_id"`
	Confidence  float64 `json:"confidence"`
	AuthHint    string  `json:"auth_hint,omitempty"`
	Profile     string  `json:"profile,omitempty"`
	Evidence    string  `json:"evidence,omitempty"`
}

const (
	compatMetadataConnector  = "compat_connector"
	compatMetadataConfidence = "compat_confidence"
	compatMetadataAuthHint   = "compat_auth_hint"
	compatMetadataProfile    = "compat_profile"
	compatMetadataEvidence   = "compat_evidence"

	defaultCompatMinConfidence = 0.60
)

func (d *Deps) HandleWebServiceCompat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.WebServiceCoordinator == nil {
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"compatible": []any{}})
		return
	}

	hostFilter := strings.TrimSpace(r.URL.Query().Get("host"))
	includeHidden := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_hidden")), "true")
	connectorFilter := normalizeCompatConnectorID(r.URL.Query().Get("connector"))

	minConfidence := defaultCompatMinConfidence
	if raw := strings.TrimSpace(r.URL.Query().Get("min_confidence")); raw != "" {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil || parsed < 0 || parsed > 1 {
			servicehttp.WriteError(w, http.StatusBadRequest, "min_confidence must be a number between 0 and 1")
			return
		}
		minConfidence = parsed
	}

	var services []agentmgr.DiscoveredWebService
	if hostFilter != "" {
		services = d.WebServiceCoordinator.ListByHost(hostFilter)
	} else {
		services = d.WebServiceCoordinator.ListAll()
	}
	if services == nil {
		services = []agentmgr.DiscoveredWebService{}
	}
	services, _ = applyWebServiceURLGrouping(services, d.ResolveWebServiceURLGroupingConfig())

	items := make([]webServiceCompatibleAPI, 0, len(services))
	for _, svc := range services {
		if !includeHidden && isHiddenWebService(svc) {
			continue
		}
		if svc.Metadata == nil {
			continue
		}

		connectorID := normalizeCompatConnectorID(svc.Metadata[compatMetadataConnector])
		if connectorID == "" {
			continue
		}
		if connectorFilter != "" && connectorID != connectorFilter {
			continue
		}

		confidence := parseCompatConfidence(svc.Metadata[compatMetadataConfidence])
		if confidence < minConfidence {
			continue
		}

		item := webServiceCompatibleAPI{
			HostAssetID: strings.TrimSpace(svc.HostAssetID),
			ServiceID:   strings.TrimSpace(svc.ID),
			ServiceName: strings.TrimSpace(svc.Name),
			ServiceURL:  strings.TrimSpace(svc.URL),
			Category:    strings.TrimSpace(svc.Category),
			Source:      strings.TrimSpace(svc.Source),
			ConnectorID: connectorID,
			Confidence:  confidence,
			AuthHint:    strings.TrimSpace(svc.Metadata[compatMetadataAuthHint]),
			Profile:     strings.TrimSpace(svc.Metadata[compatMetadataProfile]),
			Evidence:    strings.TrimSpace(svc.Metadata[compatMetadataEvidence]),
		}
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].HostAssetID != items[j].HostAssetID {
			return items[i].HostAssetID < items[j].HostAssetID
		}
		if items[i].ConnectorID != items[j].ConnectorID {
			return items[i].ConnectorID < items[j].ConnectorID
		}
		if items[i].Confidence != items[j].Confidence {
			return items[i].Confidence > items[j].Confidence
		}
		if items[i].ServiceName != items[j].ServiceName {
			return items[i].ServiceName < items[j].ServiceName
		}
		return items[i].ServiceID < items[j].ServiceID
	})

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"compatible": items})
}

func parseCompatConfidence(raw string) float64 {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0
	}
	value, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0
	}
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func normalizeCompatConnectorID(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return ""
	case "homeassistant", "home-assistant":
		return "home-assistant"
	case "proxmox":
		return "proxmox"
	case "pbs":
		return "pbs"
	case "truenas":
		return "truenas"
	case "portainer":
		return "portainer"
	case "docker":
		return "docker"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}
