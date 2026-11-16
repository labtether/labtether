package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentidentity"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/agentsettings"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
	agentspkg "github.com/labtether/labtether/internal/hubapi/agents"
	"github.com/labtether/labtether/internal/persistence"
)

// TestAgentsRoutesReachHandlersThroughRouteTable sends one signed-in admin
// request to each of the 20 agents routes through the real route table and
// auth, and checks the answer only that route's handler gives.
//
// The agents handler tests live in internal/hubapi/agents and build their
// Deps with testutil/hubdeps. This test catches a fault in cmd's own wiring
// that those tests cannot see: a route bound to the wrong handler, or
// buildAgentsDeps leaving out a store, the pending agents, the agent manager
// or cache, the audit callback, or cmd's Hub URL resolvers (the answers
// carry the external URL only cmd's resolver reads, and the device setting
// only the runtime store holds).
func TestAgentsRoutesReachHandlersThroughRouteTable(t *testing.T) {
	disableTailscaleResolutionForTest(t)
	const hubURL = "https://labtether.example.com"
	sut := newTestAPIServer(t)
	sut.externalURL = hubURL
	sut.agentCache = agentsWiringCache(t)
	sut.presenceStore = agentsWiringPresence{}
	sut.agentMgr = agentmgr.NewManager()
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "node-1", Type: "host", Name: "node-1", Source: "agent", Status: "online",
	}); err != nil {
		t.Fatalf("seed device: %v", err)
	}
	serverConn, _, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()
	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-1", "linux"))
	defer sut.agentMgr.Unregister("node-1")
	approveConn, _, cleanupApprove := createWSPairForPendingEnrollmentTest(t)
	defer cleanupApprove()
	rejectConn, _, cleanupReject := createWSPairForPendingEnrollmentTest(t)
	defer cleanupReject()
	verifiedAt := time.Now().UTC()
	sut.pendingAgents.Add(&pendingAgent{AssetID: "pending-wiring-approve", Hostname: "wiring-approve", Platform: "linux",
		DeviceFingerprint: "LT-WIRING-APPROVE", DeviceKeyAlg: agentidentity.KeyAlgorithmEd25519,
		IdentityVerified: true, IdentityVerifiedAt: &verifiedAt, Conn: approveConn})
	sut.pendingAgents.Add(&pendingAgent{AssetID: "pending-wiring-reject", Hostname: "wiring-reject", Platform: "linux", Conn: rejectConn})
	admin := createRoleSessionToken(t, sut, "admin-agents-wiring", auth.RoleAdmin)
	call := routeWiringCaller(t, sut.buildHTTPHandlers(nil, nil, nil), admin)

	var token struct {
		Token    struct{ ID string } `json:"token"`
		RawToken string              `json:"raw_token"`
	}
	created := call("/settings/enrollment", http.MethodPost, "/settings/enrollment",
		`{"label":"wiring","ttl_hours":1,"max_uses":1,"scope":"unrestricted","acknowledge_unrestricted":true}`, http.StatusCreated, `"raw_token"`)
	if err := json.Unmarshal([]byte(created), &token); err != nil || token.RawToken == "" {
		t.Fatalf("no enrollment token in %s (err %v)", created, err)
	}
	call("/api/v1/enroll", http.MethodPost, "/api/v1/enroll", `{"enrollment_token":"`+token.RawToken+`","hostname":"node-enrolled","platform":"linux"}`,
		http.StatusOK, `"hub_api_url":"`+hubURL+`"`)
	call("/settings/enrollment/", http.MethodDelete, "/settings/enrollment/"+token.Token.ID, "", http.StatusOK, `"status":"revoked"`)
	call("/api/v1/discover", http.MethodGet, "/api/v1/discover", "", http.StatusOK, `"enroll_url":"`+hubURL+`/api/v1/enroll"`)

	tokens := call("/settings/agent-tokens", http.MethodGet, "/settings/agent-tokens", "", http.StatusOK, `"asset_id":"node-enrolled"`)
	var agentTokens struct{ Tokens []struct{ ID string } }
	if err := json.Unmarshal([]byte(tokens), &agentTokens); err != nil || len(agentTokens.Tokens) != 1 {
		t.Fatalf("agent tokens %s (err %v)", tokens, err)
	}
	call("/settings/agent-tokens/", http.MethodDelete, "/settings/agent-tokens/"+agentTokens.Tokens[0].ID, "", http.StatusOK, `"status":"revoked"`)
	call("/settings/tokens/cleanup", http.MethodDelete, "/settings/tokens/cleanup", "", http.StatusOK, `"agent_deleted":1`)
	if events, err := sut.auditStore.List(20, 0); err != nil || !strings.Contains(agentsWiringJSON(t, events), `"type":"agent.token.revoked"`) {
		t.Fatalf("revoking the agent token wrote no audit event: %v %v", events, err)
	}

	call("/api/v1/agent/binary", http.MethodGet, "/api/v1/agent/binary?os=linux&arch=amd64", "", http.StatusOK, "agent-binary-for-wiring")
	call("/api/v1/agent/releases/latest", http.MethodGet, "/api/v1/agent/releases/latest?os=linux&arch=amd64", "", http.StatusOK,
		`"url":"`+hubURL+`/api/v1/agent/binary?os=linux`+"\\u0026"+`arch=amd64"`)
	call("/api/v1/agent/manifest", http.MethodGet, "/api/v1/agent/manifest", "", http.StatusOK, `"version":"0.0.0-wiring"`)
	call("/api/v1/agent/cache/refresh", http.MethodPost, "/api/v1/agent/cache/refresh", "", http.StatusOK, `"labtether-agent":"0.0.0-wiring"`)
	installWS := "'wss://labtether.example.com/ws/agent'" // the shell-quoted form only the install script has
	call("/api/v1/agent/install.sh", http.MethodGet, "/api/v1/agent/install.sh", "", http.StatusOK, installWS)
	fingerprint := strings.Repeat("ab", 32)
	call("/api/v1/agent/bootstrap.sh", http.MethodGet, "/api/v1/agent/bootstrap.sh?ca_fingerprint_sha256="+fingerprint, "", http.StatusOK, fingerprint)
	call("/install.sh", http.MethodGet, "/install.sh", "", http.StatusOK, installWS)

	call("/agents/connected", http.MethodGet, "/agents/connected", "", http.StatusOK, `"assets":["node-1"]`)
	call("/agents/presence", http.MethodGet, "/agents/presence", "", http.StatusOK, `"asset_id":"node-presence-wiring"`)
	turnURL := "turn:turn.example.com:3478" // a device setting saved in the runtime store
	if _, err := sut.runtimeStore.SaveRuntimeSettingOverrides(map[string]string{
		agentSettingStoreKey("node-1", agentsettings.SettingKeyWebRTCTURNURL): turnURL,
	}); err != nil {
		t.Fatalf("save device setting: %v", err)
	}
	call("/api/v1/agents/", http.MethodGet, "/api/v1/agents/node-1/settings", "", http.StatusOK, `"effective_value":"`+turnURL+`"`)
	call("/api/v1/agents/pending", http.MethodGet, "/api/v1/agents/pending", "", http.StatusOK, `"asset_id":"pending-wiring-reject"`)
	call("/api/v1/agents/approve", http.MethodPost, "/api/v1/agents/approve", `{"asset_id":"pending-wiring-approve"}`, http.StatusOK, `"asset_id":"wiring-approve"`)
	call("/api/v1/agents/reject", http.MethodPost, "/api/v1/agents/reject", `{"asset_id":"pending-wiring-reject"}`, http.StatusOK, `"status":"rejected"`)
	if sut.pendingAgents.Count() != 0 {
		t.Fatalf("pending agents left after approve and reject: %d", sut.pendingAgents.Count())
	}
}

// agentsWiringCache is an agent cache over a temporary folder with one
// linux/amd64 binary and a manifest with its checksum.
func agentsWiringCache(t *testing.T) *agentspkg.AgentCache {
	t.Helper()
	dir := t.TempDir()
	binary := []byte("agent-binary-for-wiring")
	if err := os.WriteFile(filepath.Join(dir, "labtether-agent-linux-amd64"), binary, 0o600); err != nil {
		t.Fatalf("write agent binary: %v", err)
	}
	manifest := fmt.Sprintf(`{"schema_version":1,"generated_at":"2026-01-01T00:00:00Z","hub_version":"test","agents":{"labtether-agent":`+
		`{"version":"0.0.0-wiring","repo":"labtether/labtether-agent","binaries":{"linux-amd64":`+
		`{"name":"labtether-agent-linux-amd64","sha256":"%x","size_bytes":%d}}}}}`, sha256.Sum256(binary), len(binary))
	if err := os.WriteFile(filepath.Join(dir, "agent-manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	cache := &agentspkg.AgentCache{RuntimeDir: dir, BakedInDir: dir}
	if err := cache.LoadManifest(); err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	return cache
}

// agentsWiringPresence lists one marked presence record. The presence route
// calls nothing else.
type agentsWiringPresence struct{ persistence.PresenceStore }

func (agentsWiringPresence) ListPresence() ([]persistence.AgentPresence, error) {
	return []persistence.AgentPresence{{AssetID: "node-presence-wiring"}}, nil
}

func agentsWiringJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return string(raw)
}
