package collectors

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/servicehttp"
)

const (
	WebServiceHealthLogSource      = "web_service_health"
	WebServiceUptimeDropThreshold  = 90.0
	webServiceUptimeDropMinChecks  = 8
	WebServiceStatusTransitionKind = "status_transition"
	WebServiceUptimeDropKind       = "uptime_drop"
)

var (
	WebServiceCleanupInterval = 60 * time.Second
	WebServiceCleanupStep     = func(d *Deps) { d.WebServiceCoordinator.CleanExpired() }
)

type webServiceAlertState struct {
	ID         string
	Name       string
	URL        string
	Status     string
	ResponseMS int
	Health     *agentmgr.WebServiceHealthSummary
}

// ProcessAgentWebServiceReport handles webservice.report messages from agents.
func (d *Deps) ProcessAgentWebServiceReport(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	if d.WebServiceCoordinator == nil {
		return
	}
	hostAssetID := strings.TrimSpace(conn.AssetID)
	previousServices := d.WebServiceCoordinator.ListByHost(hostAssetID)
	d.WebServiceCoordinator.AttachHealthSummaries(previousServices)
	previousByID := indexWebServiceAlertState(previousServices)

	d.WebServiceCoordinator.HandleReport(conn.AssetID, msg)
	if d.LogStore == nil {
		return
	}

	var report agentmgr.WebServiceReportData
	if err := json.Unmarshal(msg.Data, &report); err != nil || len(report.Services) == 0 {
		return
	}

	currentServices := d.WebServiceCoordinator.ListByHost(hostAssetID)
	d.WebServiceCoordinator.AttachHealthSummaries(currentServices)
	currentByID := indexWebServiceAlertState(currentServices)

	for _, reported := range report.Services {
		serviceID := strings.TrimSpace(reported.ID)
		if serviceID == "" {
			continue
		}
		current, ok := currentByID[serviceID]
		if !ok {
			continue
		}
		previous := previousByID[serviceID]
		d.emitWebServiceStatusTransition(hostAssetID, previous, current)
		d.emitWebServiceUptimeDrop(hostAssetID, previous, current)
	}
}

type webServiceSyncRequest struct {
	HostAssetID string `json:"host_asset_id,omitempty"`
	Host        string `json:"host,omitempty"`
}

// HandleWebServiceSync handles POST /api/v1/services/web/sync — trigger immediate rediscovery.
func (d *Deps) HandleWebServiceSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.AgentMgr == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "agent manager unavailable")
		return
	}

	targetHost := strings.TrimSpace(r.URL.Query().Get("host"))
	if targetHost == "" && r.Body != nil {
		var req webServiceSyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		targetHost = strings.TrimSpace(req.HostAssetID)
		if targetHost == "" {
			targetHost = strings.TrimSpace(req.Host)
		}
	}

	msg := agentmgr.Message{Type: agentmgr.MsgWebServiceSync}
	if targetHost != "" {
		if err := d.AgentMgr.SendToAgent(targetHost, msg); err != nil {
			if !d.AgentMgr.IsConnected(targetHost) {
				servicehttp.WriteError(w, http.StatusNotFound, "target host is not connected")
				return
			}
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to request service sync")
			return
		}
		servicehttp.WriteJSON(w, http.StatusAccepted, map[string]any{
			"requested": []string{targetHost},
			"queued":    1,
			"sent_to":   []string{targetHost},
		})
		return
	}

	targets := d.AgentMgr.ConnectedAssets()
	if len(targets) == 0 {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "no connected agents")
		return
	}

	sentTo := make([]string, 0, len(targets))
	failed := make(map[string]string)
	for _, assetID := range targets {
		if err := d.AgentMgr.SendToAgent(assetID, msg); err != nil {
			failed[assetID] = err.Error()
			continue
		}
		sentTo = append(sentTo, assetID)
	}
	if len(sentTo) == 0 {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to request service sync")
		return
	}

	payload := map[string]any{
		"requested": targets,
		"queued":    len(sentTo),
		"sent_to":   sentTo,
	}
	if len(failed) > 0 {
		payload["failed"] = failed
	}
	servicehttp.WriteJSON(w, http.StatusAccepted, payload)
}

// RunWebServiceCleanup periodically removes stale web service entries for
// disconnected agents whose TTL has expired.
func (d *Deps) RunWebServiceCleanup(ctx context.Context) {
	if d.WebServiceCoordinator == nil {
		return
	}
	ticker := time.NewTicker(WebServiceCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			WebServiceCleanupStep(d)
		}
	}
}

func indexWebServiceAlertState(services []agentmgr.DiscoveredWebService) map[string]webServiceAlertState {
	out := make(map[string]webServiceAlertState, len(services))
	for i := range services {
		serviceID := strings.TrimSpace(services[i].ID)
		if serviceID == "" {
			continue
		}
		out[serviceID] = webServiceAlertState{
			ID:         serviceID,
			Name:       strings.TrimSpace(services[i].Name),
			URL:        strings.TrimSpace(services[i].URL),
			Status:     normalizeWebServiceAlertStatus(services[i].Status),
			ResponseMS: services[i].ResponseMs,
			Health:     services[i].Health,
		}
	}
	return out
}

func normalizeWebServiceAlertStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "up":
		return "up"
	case "down":
		return "down"
	default:
		return "unknown"
	}
}

func (d *Deps) emitWebServiceStatusTransition(
	hostAssetID string,
	previous webServiceAlertState,
	current webServiceAlertState,
) {
	currentStatus := normalizeWebServiceAlertStatus(current.Status)
	previousStatus := normalizeWebServiceAlertStatus(previous.Status)
	if previous.ID == "" {
		if currentStatus != "down" {
			return
		}
	} else if currentStatus == previousStatus {
		return
	}

	level := "info"
	if currentStatus == "down" {
		level = "warn"
	}

	fields := map[string]string{
		"event_kind":    WebServiceStatusTransitionKind,
		"host_asset_id": strings.TrimSpace(hostAssetID),
		"service_id":    current.ID,
		"service_name":  current.Name,
		"service_url":   current.URL,
		"status":        currentStatus,
		"response_ms":   strconv.Itoa(current.ResponseMS),
	}
	if previous.ID != "" {
		fields["previous_status"] = previousStatus
	}

	_ = d.LogStore.AppendEvent(logs.Event{
		AssetID:   hostAssetID,
		Source:    WebServiceHealthLogSource,
		Level:     level,
		Message:   "web service status changed",
		Fields:    fields,
		Timestamp: time.Now().UTC(),
	})
}

func (d *Deps) emitWebServiceUptimeDrop(
	hostAssetID string,
	previous webServiceAlertState,
	current webServiceAlertState,
) {
	if current.Health == nil {
		return
	}
	currentChecks := current.Health.Checks
	currentUptime := current.Health.UptimePercent
	if currentChecks < webServiceUptimeDropMinChecks || currentUptime >= WebServiceUptimeDropThreshold {
		return
	}

	previousBelowThreshold := false
	if previous.Health != nil {
		previousBelowThreshold = previous.Health.Checks >= webServiceUptimeDropMinChecks
		previousBelowThreshold = previousBelowThreshold && previous.Health.UptimePercent < WebServiceUptimeDropThreshold
	}
	if previousBelowThreshold {
		return
	}

	fields := map[string]string{
		"event_kind":          WebServiceUptimeDropKind,
		"host_asset_id":       strings.TrimSpace(hostAssetID),
		"service_id":          current.ID,
		"service_name":        current.Name,
		"service_url":         current.URL,
		"status":              normalizeWebServiceAlertStatus(current.Status),
		"uptime_percent":      strconv.FormatFloat(currentUptime, 'f', 2, 64),
		"threshold_percent":   strconv.FormatFloat(WebServiceUptimeDropThreshold, 'f', 0, 64),
		"checks":              strconv.Itoa(currentChecks),
		"minimum_checks":      strconv.Itoa(webServiceUptimeDropMinChecks),
		"health_window":       current.Health.Window,
		"last_status_change":  strings.TrimSpace(current.Health.LastChangeAt),
		"last_health_checked": strings.TrimSpace(current.Health.LastCheckedAt),
	}

	_ = d.LogStore.AppendEvent(logs.Event{
		AssetID:   hostAssetID,
		Source:    WebServiceHealthLogSource,
		Level:     "warn",
		Message:   "web service rolling uptime dropped below threshold",
		Fields:    fields,
		Timestamp: time.Now().UTC(),
	})
}
