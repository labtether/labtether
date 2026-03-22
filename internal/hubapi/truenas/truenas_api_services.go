package truenas

import (
	"context"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/servicehttp"
)

type TrueNASServiceEntry struct {
	ID      string `json:"id,omitempty"`
	Service string `json:"service"`
	State   string `json:"state"`
	Enabled bool   `json:"enabled"`
}

type TrueNASServicesResponse struct {
	AssetID   string                `json:"asset_id"`
	Services  []TrueNASServiceEntry `json:"services"`
	Warnings  []string              `json:"warnings,omitempty"`
	FetchedAt string                `json:"fetched_at"`
}

type TrueNASServiceActionResponse struct {
	AssetID     string   `json:"asset_id"`
	ServiceName string   `json:"service_name"`
	Action      string   `json:"action"`
	Message     string   `json:"message"`
	Warnings    []string `json:"warnings,omitempty"`
	FetchedAt   string   `json:"fetched_at"`
}

func MapTrueNASServiceEntry(svc map[string]any) TrueNASServiceEntry {
	enabled := false
	if v, ok := shared.ParseAnyBoolLoose(svc["enable"]); ok {
		enabled = v
	}
	return TrueNASServiceEntry{
		ID:      strings.TrimSpace(shared.CollectorAnyString(svc["id"])),
		Service: strings.TrimSpace(shared.CollectorAnyString(svc["service"])),
		State:   strings.ToLower(strings.TrimSpace(shared.CollectorAnyString(svc["state"]))),
		Enabled: enabled,
	}
}

func (d *Deps) HandleTrueNASServices(ctx context.Context, w http.ResponseWriter, r *http.Request, asset assets.Asset, runtime *TruenasRuntime, subParts []string) {
	// GET  /truenas/assets/{id}/services                     → list services
	// POST /truenas/assets/{id}/services/{name}/start        → start service
	// POST /truenas/assets/{id}/services/{name}/stop         → stop service
	// POST /truenas/assets/{id}/services/{name}/restart      → restart service
	// PUT  /truenas/assets/{id}/services/{name}/enable       → enable/disable service

	if len(subParts) == 0 {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		rawServices := make([]map[string]any, 0, 16)
		warnings := make([]string, 0, 2)
		if err := CallTrueNASQueryWithRetries(ctx, runtime.Client, "service.query", &rawServices); err != nil {
			warnings = AppendTrueNASWarning(warnings, "services unavailable: "+err.Error())
			rawServices = nil
		}
		services := make([]TrueNASServiceEntry, 0, len(rawServices))
		for _, svc := range rawServices {
			services = append(services, MapTrueNASServiceEntry(svc))
		}
		servicehttp.WriteJSON(w, http.StatusOK, TrueNASServicesResponse{
			AssetID:   strings.TrimSpace(asset.ID),
			Services:  services,
			Warnings:  warnings,
			FetchedAt: time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	serviceName := strings.TrimSpace(subParts[0])
	if serviceName == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "service name is required")
		return
	}
	if len(subParts) < 2 {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown service action")
		return
	}

	serviceAction := strings.TrimSpace(subParts[1])
	switch serviceAction {
	case "start", "stop", "restart":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.RequireAdminAuth(w, r) {
			return
		}
		method := "service." + serviceAction
		if err := CallTrueNASMethodWithRetries(ctx, runtime.Client, method, []any{serviceName}, nil); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to "+serviceAction+" service: "+err.Error())
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, TrueNASServiceActionResponse{
			AssetID:     strings.TrimSpace(asset.ID),
			ServiceName: serviceName,
			Action:      serviceAction,
			Message:     "service " + serviceName + " " + serviceAction + "ed",
			FetchedAt:   time.Now().UTC().Format(time.RFC3339),
		})
	case "enable":
		if r.Method != http.MethodPut {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.RequireAdminAuth(w, r) {
			return
		}
		var body struct {
			Enable bool `json:"enable"`
		}
		body.Enable = true // default to enabling
		if r.Body != nil && r.ContentLength != 0 {
			if err := shared.DecodeJSONBody(w, r, &body); err != nil {
				servicehttp.WriteError(w, http.StatusBadRequest, "invalid JSON payload")
				return
			}
		}
		// service.update params: [id, {enable: bool}]
		// We need the numeric service ID. Look it up by name first.
		rawServices := make([]map[string]any, 0, 16)
		if err := CallTrueNASQueryWithRetries(ctx, runtime.Client, "service.query", &rawServices); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to query services: "+err.Error())
			return
		}
		serviceID := ""
		for _, svc := range rawServices {
			name := strings.TrimSpace(shared.CollectorAnyString(svc["service"]))
			if strings.EqualFold(name, serviceName) {
				serviceID = strings.TrimSpace(shared.CollectorAnyString(svc["id"]))
				break
			}
		}
		if serviceID == "" {
			servicehttp.WriteError(w, http.StatusNotFound, "service not found: "+serviceName)
			return
		}
		if err := CallTrueNASMethodWithRetries(ctx, runtime.Client, "service.update", []any{serviceID, map[string]any{"enable": body.Enable}}, nil); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to update service: "+err.Error())
			return
		}
		action := "enabled"
		if !body.Enable {
			action = "disabled"
		}
		servicehttp.WriteJSON(w, http.StatusOK, TrueNASServiceActionResponse{
			AssetID:     strings.TrimSpace(asset.ID),
			ServiceName: serviceName,
			Action:      "enable",
			Message:     "service " + serviceName + " " + action,
			FetchedAt:   time.Now().UTC().Format(time.RFC3339),
		})
	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown service action: "+serviceAction)
	}
}
