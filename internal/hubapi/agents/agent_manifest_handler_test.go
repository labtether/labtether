package agents

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHandleAgentManifest_ReturnsManifest(t *testing.T) {
	cache := &AgentCache{}
	cache.SetManifest(&AgentManifest{
		SchemaVersion: 1,
		HubVersion:    "v2026.1",
		Agents: map[string]AgentEntry{
			"labtether-agent": {Version: "v2026.1"},
		},
	})
	d := &Deps{AgentCache: cache}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/manifest", nil)
	w := httptest.NewRecorder()
	d.HandleAgentManifest(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var m AgentManifest
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m.HubVersion != "v2026.1" {
		t.Errorf("hub_version = %q", m.HubVersion)
	}
}

func TestHandleAgentManifest_NoManifest(t *testing.T) {
	cache := &AgentCache{}
	d := &Deps{AgentCache: cache}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/manifest", nil)
	w := httptest.NewRecorder()
	d.HandleAgentManifest(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestHandleAgentCacheRefresh_Success(t *testing.T) {
	dir := t.TempDir()
	writeManifestFile(t, dir, "v2026.1")
	cache := &AgentCache{RuntimeDir: dir, BakedInDir: dir, refreshCooldown: 0}
	cache.SetManifest(&AgentManifest{Agents: map[string]AgentEntry{
		"labtether-agent": {Version: "v2026.0"},
	}})
	d := &Deps{AgentCache: cache}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/cache/refresh", nil)
	w := httptest.NewRecorder()
	d.HandleAgentCacheRefresh(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleAgentCacheRefresh_Cooldown(t *testing.T) {
	dir := t.TempDir()
	writeManifestFile(t, dir, "v2026.1")
	cache := &AgentCache{RuntimeDir: dir, BakedInDir: dir, refreshCooldown: 1 * time.Hour}
	cache.SetManifest(&AgentManifest{})
	cache.TryRefresh() // consume the first attempt

	d := &Deps{AgentCache: cache}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/cache/refresh", nil)
	w := httptest.NewRecorder()
	d.HandleAgentCacheRefresh(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}
}

func writeManifestFile(t *testing.T, dir, version string) {
	t.Helper()
	data := fmt.Sprintf(`{"schema_version":1,"generated_at":"2026-03-24T12:00:00Z","hub_version":"%s","agents":{"labtether-agent":{"version":"%s","repo":"labtether/labtether-agent","binaries":{}}}}`, version, version)
	if err := os.WriteFile(filepath.Join(dir, "agent-manifest.json"), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}
