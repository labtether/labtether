package agents

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentcore"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

type AgentSettingsPatchRequest struct {
	Values map[string]string `json:"values"`
}

type AgentSettingsResetRequest struct {
	Keys []string `json:"keys"`
}

type AgentSettingsDockerTestRequest struct {
	Endpoint string `json:"endpoint,omitempty"`
	Enabled  string `json:"enabled,omitempty"`
}

type AgentSettingsUpdateRequest struct {
	Force bool `json:"force,omitempty"`
}

func (d *Deps) HandleAgentSettingsRoutes(w http.ResponseWriter, r *http.Request) {
	const prefix = "/api/v1/agents/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	trimmed := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) < 2 || strings.TrimSpace(parts[1]) != "settings" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if len(parts) > 3 {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	assetID, err := url.PathUnescape(strings.TrimSpace(parts[0]))
	if err != nil || strings.TrimSpace(assetID) == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid asset id")
		return
	}

	action := ""
	if len(parts) >= 3 {
		action = strings.TrimSpace(parts[2])
	}

	switch action {
	case "":
		switch r.Method {
		case http.MethodGet:
			d.HandleAgentSettingsGet(w, r, assetID)
		case http.MethodPatch:
			d.HandleAgentSettingsPatch(w, r, assetID)
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "reset":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		d.HandleAgentSettingsReset(w, r, assetID)
	case "test-docker":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		d.HandleAgentSettingsDockerTest(w, r, assetID)
	case "history":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		d.HandleAgentSettingsHistory(w, r, assetID)
	case "update-agent":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		d.HandleAgentSettingsUpdateAgent(w, r, assetID)
	default:
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
	}
}

func (d *Deps) HandleAgentSettingsGet(w http.ResponseWriter, _ *http.Request, assetID string) {
	payload, err := d.BuildAgentSettingsPayload(assetID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, payload)
}

func (d *Deps) HandleAgentSettingsPatch(w http.ResponseWriter, r *http.Request, assetID string) {
	if !d.EnforceRateLimit(w, r, "agent.settings.patch", 60, time.Minute) {
		return
	}

	if d.RuntimeStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "runtime settings unavailable")
		return
	}

	var req AgentSettingsPatchRequest
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid settings payload")
		return
	}
	normalized, err := NormalizeAgentSettingValues(req.Values, true)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(normalized) == 0 {
		servicehttp.WriteError(w, http.StatusBadRequest, "settings values are required")
		return
	}

	storeValues := make(map[string]string, len(normalized))
	for key, value := range normalized {
		storeValues[AgentSettingStoreKey(assetID, key)] = value
	}
	if _, err := d.RuntimeStore.SaveRuntimeSettingOverrides(storeValues); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to save agent settings")
		return
	}

	effective, err := d.CollectEffectiveAgentSettingValues(assetID)
	if err == nil {
		d.PushAgentSettingsApply(assetID, effective)
	}

	payload, err := d.BuildAgentSettingsPayload(assetID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, payload)
}

func (d *Deps) HandleAgentSettingsReset(w http.ResponseWriter, r *http.Request, assetID string) {
	if !d.EnforceRateLimit(w, r, "agent.settings.reset", 60, time.Minute) {
		return
	}
	if d.RuntimeStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "runtime settings unavailable")
		return
	}

	var req AgentSettingsResetRequest
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid reset payload")
		return
	}

	var keys []string
	if len(req.Keys) == 0 {
		definitions := agentcore.AgentSettingDefinitions()
		keys = make([]string, 0, len(definitions))
		for _, definition := range definitions {
			keys = append(keys, AgentSettingStoreKey(assetID, definition.Key))
		}
	} else {
		for _, raw := range req.Keys {
			key := strings.TrimSpace(raw)
			if _, ok := agentcore.AgentSettingDefinitionByKey(key); !ok {
				servicehttp.WriteError(w, http.StatusBadRequest, fmt.Sprintf("unknown setting key: %s", key))
				return
			}
			keys = append(keys, AgentSettingStoreKey(assetID, key))
		}
	}

	if err := d.RuntimeStore.DeleteRuntimeSettingOverrides(keys); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to reset settings")
		return
	}

	effective, err := d.CollectEffectiveAgentSettingValues(assetID)
	if err == nil {
		d.PushAgentSettingsApply(assetID, effective)
	}

	payload, err := d.BuildAgentSettingsPayload(assetID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, payload)
}

func (d *Deps) HandleAgentSettingsDockerTest(w http.ResponseWriter, r *http.Request, assetID string) {
	if !d.EnforceRateLimit(w, r, "agent.settings.testdocker", 30, time.Minute) {
		return
	}
	var req AgentSettingsDockerTestRequest
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid docker test payload")
		return
	}

	effective, err := d.CollectEffectiveAgentSettingValues(assetID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to resolve effective settings")
		return
	}
	if raw := strings.TrimSpace(req.Enabled); raw != "" {
		normalized, err := agentcore.NormalizeAgentSettingValue(agentcore.SettingKeyDockerEnabled, raw)
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		effective[agentcore.SettingKeyDockerEnabled] = normalized
	}
	if raw := strings.TrimSpace(req.Endpoint); raw != "" {
		normalized, err := agentcore.NormalizeAgentSettingValue(agentcore.SettingKeyDockerEndpoint, raw)
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		effective[agentcore.SettingKeyDockerEndpoint] = normalized
	}

	mode := strings.TrimSpace(strings.ToLower(effective[agentcore.SettingKeyDockerEnabled]))
	if mode == "false" {
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"message": "docker is disabled by configuration",
		})
		return
	}

	endpoint := strings.TrimSpace(effective[agentcore.SettingKeyDockerEndpoint])
	if endpoint == "" {
		endpoint = "/var/run/docker.sock"
	}

	if d.AgentMgr == nil || !d.AgentMgr.IsConnected(assetID) {
		servicehttp.WriteError(w, http.StatusConflict, "agent must be connected to run docker test")
		return
	}

	command := DockerConnectivityTestCommand(endpoint)
	jobID := shared.GenerateRequestID()
	result := d.ExecuteViaAgent(terminal.CommandJob{
		JobID:     jobID,
		SessionID: jobID,
		CommandID: jobID,
		Target:    assetID,
		Command:   command,
	})
	ok := strings.EqualFold(strings.TrimSpace(result.Status), "succeeded")
	status := http.StatusOK
	if !ok {
		status = http.StatusBadRequest
	}
	servicehttp.WriteJSON(w, status, map[string]any{
		"ok":       ok,
		"status":   result.Status,
		"output":   result.Output,
		"endpoint": endpoint,
	})
}

func (d *Deps) HandleAgentSettingsHistory(w http.ResponseWriter, _ *http.Request, assetID string) {
	state, ok := d.GetAgentSettingsRuntimeState(assetID)
	events := []map[string]any{}
	if ok {
		events = append(events, map[string]any{
			"asset_id":         assetID,
			"status":           state.Status,
			"revision":         state.Revision,
			"last_error":       state.LastError,
			"updated_at":       state.UpdatedAt.Format(time.RFC3339),
			"applied_at":       ZeroTimeToRFC3339(state.AppliedAt),
			"restart_required": state.RestartRequired,
			"fingerprint":      state.Fingerprint,
		})
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (d *Deps) HandleAgentSettingsUpdateAgent(w http.ResponseWriter, r *http.Request, assetID string) {
	if !d.EnforceRateLimit(w, r, "agent.settings.update", 20, time.Minute) {
		return
	}
	if d.AgentMgr == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "agent manager unavailable")
		return
	}

	var req AgentSettingsUpdateRequest
	if err := shared.DecodeJSONBody(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid update payload")
		return
	}
	if !d.AgentMgr.IsConnected(assetID) {
		servicehttp.WriteError(w, http.StatusConflict, "agent must be connected to update")
		return
	}

	jobID := shared.GenerateRequestID()
	result := d.ExecuteUpdateViaAgent(jobID, assetID, "self", nil, d.DefaultUpdateAgentTimeout, req.Force)

	status := strings.ToLower(strings.TrimSpace(result.Status))
	if status == "" {
		status = "failed"
	}

	output := strings.TrimSpace(result.Output)
	summary := d.SummarizeUpdateOutput(output)
	if summary == "" {
		if status == "succeeded" {
			summary = "agent self-update completed"
		} else {
			summary = "agent self-update failed"
		}
	}

	httpStatus := http.StatusOK
	if status != "succeeded" {
		httpStatus = http.StatusBadGateway
		lowered := strings.ToLower(output)
		switch {
		case strings.Contains(lowered, "timed out"):
			httpStatus = http.StatusGatewayTimeout
		case strings.Contains(lowered, "not connected"):
			httpStatus = http.StatusConflict
		}
	}

	if d.LogStore != nil {
		level := shared.MapCommandLevel(status)
		if err := d.LogStore.AppendEvent(logs.Event{
			ID:        fmt.Sprintf("log_agent_update_%s", jobID),
			AssetID:   assetID,
			Source:    "agent.settings",
			Level:     level,
			Message:   fmt.Sprintf("agent self-update %s", status),
			Timestamp: time.Now().UTC(),
			Fields: map[string]string{
				"job_id":  jobID,
				"force":   fmt.Sprintf("%t", req.Force),
				"summary": summary,
			},
		}); err != nil {
			// Non-fatal: update result should still be returned to the caller.
		}
	}

	servicehttp.WriteJSON(w, httpStatus, map[string]any{
		"ok":                          status == "succeeded",
		"job_id":                      jobID,
		"status":                      status,
		"summary":                     summary,
		"output":                      output,
		"force":                       req.Force,
		"agent_disconnected_expected": status == "succeeded",
	})
}

func DockerConnectivityTestCommand(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		endpoint = "/var/run/docker.sock"
	}
	if strings.HasPrefix(endpoint, "/") {
		return fmt.Sprintf(
			"if command -v curl >/dev/null 2>&1; then curl -fsS --max-time 5 --unix-socket %q http://localhost/_ping; elif command -v docker >/dev/null 2>&1; then docker --host %q version >/dev/null 2>&1; else echo curl-or-docker-required; exit 1; fi",
			endpoint,
			"unix://"+endpoint,
		)
	}
	if path, ok := TrimUnixEndpointScheme(endpoint); ok {
		return fmt.Sprintf(
			"if command -v curl >/dev/null 2>&1; then curl -fsS --max-time 5 --unix-socket %q http://localhost/_ping; elif command -v docker >/dev/null 2>&1; then docker --host %q version >/dev/null 2>&1; else echo curl-or-docker-required; exit 1; fi",
			path,
			endpoint,
		)
	}
	endpoint = strings.TrimRight(endpoint, "/")
	return fmt.Sprintf(
		"if command -v curl >/dev/null 2>&1; then curl -fsS --max-time 5 %q; elif command -v docker >/dev/null 2>&1; then docker --host %q version >/dev/null 2>&1; else echo curl-or-docker-required; exit 1; fi",
		endpoint+"/_ping",
		endpoint,
	)
}

func TrimUnixEndpointScheme(endpoint string) (string, bool) {
	trimmed := strings.TrimSpace(endpoint)
	const prefix = "unix://"
	if len(trimmed) < len(prefix) {
		return trimmed, false
	}
	if strings.EqualFold(trimmed[:len(prefix)], prefix) {
		return strings.TrimSpace(trimmed[len(prefix):]), true
	}
	return trimmed, false
}
