package collectors

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubcollector"
)

type unconfiguredHealthConnector struct{ id string }

func (c unconfiguredHealthConnector) ID() string          { return c.id }
func (c unconfiguredHealthConnector) DisplayName() string { return c.id }
func (c unconfiguredHealthConnector) Capabilities() connectorsdk.Capabilities {
	return connectorsdk.Capabilities{}
}
func (c unconfiguredHealthConnector) TestConnection(context.Context) (connectorsdk.Health, error) {
	return connectorsdk.Health{Status: "failed", Message: c.id + " connector is not configured"}, nil
}
func (c unconfiguredHealthConnector) Discover(context.Context) ([]connectorsdk.Asset, error) {
	return []connectorsdk.Asset{}, nil
}
func (c unconfiguredHealthConnector) Actions() []connectorsdk.ActionDescriptor { return nil }
func (c unconfiguredHealthConnector) ExecuteAction(context.Context, string, connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	return connectorsdk.ActionResult{Status: "failed"}, nil
}

// A connection saved in the console was reported as "not configured" by the
// connector health route (and MCP connectors_health), which looked only at
// the Hub's environment settings.
func TestConnectorHealthReportsConnectionsSavedInTheConsole(t *testing.T) {
	collected := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	store := &hubCollectorHandlerStore{collector: hubcollector.Collector{
		ID: "collector-1", AssetID: "proxmox-cluster-lab", CollectorType: "homeassistant", Enabled: true,
		LastStatus: "ok", LastCollectedAt: &collected,
	}}
	registry := connectorsdk.NewRegistry()
	registry.Register(unconfiguredHealthConnector{id: "home-assistant"})
	registry.Register(unconfiguredHealthConnector{id: "proxmox"})
	deps := newHubCollectorHandlerDeps(store)
	deps.ConnectorRegistry = registry

	health := func(id string) map[string]any {
		rec := httptest.NewRecorder()
		deps.HandleConnectorActions(rec, httptest.NewRequest(http.MethodGet, "/connectors/"+id+"/health", nil))
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("%s health = %d %s", id, rec.Code, rec.Body.String())
		}
		return body
	}
	if got := health("home-assistant"); got["status"] != "ok" || len(got["connections"].([]any)) != 1 {
		t.Fatalf("saved Home Assistant connection: health = %v, want ok with its connection", got)
	}
	if got := health("proxmox"); got["status"] != "failed" || got["message"] != "proxmox connector is not configured" {
		t.Fatalf("no saved Proxmox connection: health = %v, want the environment connector's answer", got)
	}

	store.collector.LastStatus, store.collector.LastError = "error", "Home Assistant refused the token."
	if got := health("home-assistant"); got["status"] != "failed" {
		t.Fatalf("failing saved connection: health = %v, want failed", got)
	}
}
