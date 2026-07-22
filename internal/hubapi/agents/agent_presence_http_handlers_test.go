package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/persistence"
)

type staticPresenceStore struct {
	records []persistence.AgentPresence
}

func (s *staticPresenceStore) UpsertPresence(p persistence.AgentPresence) error {
	s.records = append(s.records, p)
	return nil
}
func (s *staticPresenceStore) UpdateHeartbeat(string, time.Time) error { return nil }
func (s *staticPresenceStore) UpdateHeartbeatForSession(string, string, time.Time) (bool, error) {
	return true, nil
}
func (s *staticPresenceStore) UpdatePresenceMetadata(string, map[string]any) error { return nil }
func (s *staticPresenceStore) UpdatePresenceMetadataForSession(string, string, map[string]any) (bool, error) {
	return true, nil
}
func (s *staticPresenceStore) DeletePresence(string) error { return nil }
func (s *staticPresenceStore) DeletePresenceForSession(string, string) (bool, error) {
	return true, nil
}
func (s *staticPresenceStore) ListPresence() ([]persistence.AgentPresence, error) {
	return append([]persistence.AgentPresence(nil), s.records...), nil
}
func (s *staticPresenceStore) GetStalePresence(time.Time) ([]persistence.AgentPresence, error) {
	return nil, nil
}
func (s *staticPresenceStore) UpdateAssetTransportType(string, string) error { return nil }

func restrictedRequest(path string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	ctx := apiv2.ContextWithAllowedAssets(context.Background(), []string{"srv1"})
	return req.WithContext(ctx)
}

func TestHandleConnectedAgentsFiltersAssetAllowlist(t *testing.T) {
	mgr := agentmgr.NewManager()
	mgr.Register(&agentmgr.AgentConn{AssetID: "srv1", Platform: "linux"})
	mgr.Register(&agentmgr.AgentConn{AssetID: "srv2", Platform: "windows"})
	d := &Deps{AgentMgr: mgr}

	rec := httptest.NewRecorder()
	d.HandleConnectedAgents(rec, restrictedRequest("/agents/connected"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Count      int                           `json:"count"`
		Assets     []string                      `json:"assets"`
		AssetsInfo []agentmgr.ConnectedAssetInfo `json:"assetsInfo"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode connected response: %v", err)
	}
	if payload.Count != 1 || len(payload.Assets) != 1 || payload.Assets[0] != "srv1" {
		t.Fatalf("connected response leaked disallowed assets: %#v", payload)
	}
	if len(payload.AssetsInfo) != 1 || payload.AssetsInfo[0].ID != "srv1" {
		t.Fatalf("connected metadata leaked disallowed assets: %#v", payload.AssetsInfo)
	}
}

func TestHandleAgentPresenceFiltersAssetAllowlist(t *testing.T) {
	d := &Deps{PresenceStore: &staticPresenceStore{records: []persistence.AgentPresence{
		{AssetID: "srv1", SessionID: "allowed"},
		{AssetID: "srv2", SessionID: "denied"},
	}}}

	rec := httptest.NewRecorder()
	d.HandleAgentPresence(rec, restrictedRequest("/agents/presence"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Count    int                         `json:"count"`
		Presence []persistence.AgentPresence `json:"presence"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode presence response: %v", err)
	}
	if payload.Count != 1 || len(payload.Presence) != 1 || payload.Presence[0].AssetID != "srv1" {
		t.Fatalf("presence response leaked disallowed assets: %#v", payload)
	}
}
