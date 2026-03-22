package collectors

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/modelmap"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

func (d *Deps) HandleListConnectors(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/connectors" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"connectors": d.ConnectorRegistry.List(),
	})
}

func (d *Deps) HandleConnectorActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/connectors/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "connector path not found")
		return
	}

	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		servicehttp.WriteError(w, http.StatusNotFound, "invalid connector path")
		return
	}

	connectorID := parts[0]
	resolvedConnectorID := normalizeConnectorID(connectorID)
	connector, ok := d.ConnectorRegistry.Get(resolvedConnectorID)
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "connector not registered")
		return
	}
	if resolvedConnectorID == "portainer" {
		d.HandlePortainerConnectorActions(w, r, connector, parts)
		return
	}

	switch {
	case len(parts) == 2 && parts[1] == "test":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.EnforceRateLimit(w, r, "connector."+resolvedConnectorID+".test", 12, time.Minute) {
			return
		}
		switch resolvedConnectorID {
		case "proxmox":
			d.HandleProxmoxConnectorTest(w, r)
		case "pbs":
			d.HandlePBSConnectorTest(w, r)
		case "truenas":
			d.HandleTrueNASConnectorTest(w, r)
		case "docker":
			d.HandleDockerConnectorTest(w, r)
		case "home-assistant":
			d.HandleHomeAssistantConnectorTest(w, r)
		default:
			servicehttp.WriteError(w, http.StatusNotFound, "unsupported connector test endpoint")
		}
		return
	case len(parts) == 2 && parts[1] == "discover":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		assets, err := connector.Discover(r.Context())
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "discover failed")
			return
		}
		canonicalAssets := modelmap.CanonicalizeConnectorAssets(connectorID, assets)
		d.PersistCanonicalConnectorSnapshot(resolvedConnectorID, "", connector.DisplayName(), "", connector, canonicalAssets)
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"assets":          canonicalAssets,
			"relationships":   modelmap.SynthesizeResourceRelationships(connectorID, canonicalAssets),
			"capability_sets": modelmap.SynthesizeCapabilitySets(connector, canonicalAssets),
		})
	case len(parts) == 2 && parts[1] == "health":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		health, err := connector.TestConnection(r.Context())
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "health check failed")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, health)
	case len(parts) == 2 && parts[1] == "actions":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"actions": modelmap.CanonicalizeActionDescriptors(connector.Actions())})
	case len(parts) == 4 && parts[1] == "actions" && parts[3] == "execute":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var req connectorsdk.ActionRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil && err != io.EOF {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid action payload")
			return
		}

		resolvedActionID := modelmap.ResolveActionID(parts[2], req.TargetID, connector.Actions())

		if resolvedConnectorID == "proxmox" {
			result, err := d.ExecuteProxmoxActionDirect(r.Context(), resolvedActionID, req)
			if err != nil {
				servicehttp.WriteError(w, http.StatusBadGateway, "action execution failed")
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, result)
			return
		}

		result, err := connector.ExecuteAction(r.Context(), resolvedActionID, req)
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "action execution failed")
			return
		}
		status := http.StatusOK
		if strings.EqualFold(result.Status, "failed") {
			status = http.StatusBadRequest
		}
		servicehttp.WriteJSON(w, status, map[string]any{"result": result})
	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown connector action")
	}
}

func normalizeConnectorID(connectorID string) string {
	switch strings.TrimSpace(strings.ToLower(connectorID)) {
	case "homeassistant":
		return "home-assistant"
	default:
		return connectorID
	}
}

func (d *Deps) HandlePortainerConnectorActions(w http.ResponseWriter, r *http.Request, connector connectorsdk.Connector, parts []string) {
	switch {
	case len(parts) == 2 && parts[1] == "test":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.EnforceRateLimit(w, r, "connector.portainer.test", 12, time.Minute) {
			return
		}
		d.HandlePortainerConnectorTest(w, r)
	case len(parts) == 2 && parts[1] == "discover":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		assets, err := connector.Discover(r.Context())
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "discover failed")
			return
		}
		canonicalAssets := modelmap.CanonicalizeConnectorAssets("portainer", assets)
		d.PersistCanonicalConnectorSnapshot("portainer", "", connector.DisplayName(), "", connector, canonicalAssets)
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"assets":          canonicalAssets,
			"relationships":   modelmap.SynthesizeResourceRelationships("portainer", canonicalAssets),
			"capability_sets": modelmap.SynthesizeCapabilitySets(connector, canonicalAssets),
		})
	case len(parts) == 2 && parts[1] == "health":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		health, err := connector.TestConnection(r.Context())
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "health check failed")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, health)
	case len(parts) == 2 && parts[1] == "actions":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"actions": modelmap.CanonicalizeActionDescriptors(connector.Actions())})
	case len(parts) == 4 && parts[1] == "actions" && parts[3] == "execute":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var req connectorsdk.ActionRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil && err != io.EOF {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid action payload")
			return
		}

		resolvedActionID := modelmap.ResolveActionID(parts[2], req.TargetID, connector.Actions())
		result, err := connector.ExecuteAction(r.Context(), resolvedActionID, req)
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "action execution failed")
			return
		}
		status := http.StatusOK
		if strings.EqualFold(result.Status, "failed") {
			status = http.StatusBadRequest
		}
		servicehttp.WriteJSON(w, status, map[string]any{"result": result})
	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown connector action")
	}
}
