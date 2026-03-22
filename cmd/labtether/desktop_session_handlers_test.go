package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/terminal"
)

func TestHandleDesktopSessionsRejectsUnknownAssetTarget(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodPost, "/desktop/sessions", bytes.NewBufferString(`{"target":"unknown-node"}`))
	rec := httptest.NewRecorder()
	sut.handleDesktopSessions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown desktop target, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "managed asset") {
		t.Fatalf("expected managed-asset validation error, got %s", rec.Body.String())
	}
}

func TestHandleDesktopSessionsAcceptsManagedAssetTarget(t *testing.T) {
	sut := newTestAPIServer(t)
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "desktop-node-01",
		Type:    "node",
		Name:    "Desktop Node",
		Source:  "manual",
		Status:  "online",
	}); err != nil {
		t.Fatalf("failed to seed asset: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/desktop/sessions", bytes.NewBufferString(`{"target":"desktop-node-01"}`))
	rec := httptest.NewRecorder()
	sut.handleDesktopSessions(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for managed asset target, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDesktopSessionsAppliesInteractivePolicy(t *testing.T) {
	sut := newTestAPIServer(t)
	cfg := policy.DefaultEvaluatorConfig()
	cfg.InteractiveEnabled = false
	sut.policyState = newPolicyRuntimeState(cfg)
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "desktop-node-policy",
		Type:    "node",
		Name:    "Desktop Node Policy",
		Source:  "manual",
		Status:  "online",
	}); err != nil {
		t.Fatalf("failed to seed asset: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/desktop/sessions", bytes.NewBufferString(`{"target":"desktop-node-policy"}`))
	rec := httptest.NewRecorder()
	sut.handleDesktopSessions(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when interactive mode is disabled, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "interactive mode disabled") {
		t.Fatalf("expected interactive policy denial, got %s", rec.Body.String())
	}
}

func TestHandleDesktopStreamTicketIssuesAgentVNCPasswordWithoutQueryLeak(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()
	sut.agentMgr.Register(&agentmgr.AgentConn{AssetID: "desktop-node-01"})
	sut.desktopDeps = nil // reset cached deps to pick up new agentMgr
	sut.setDesktopSessionOptions("sess-vnc-auth", desktopSessionOptions{
		Protocol: "vnc",
		Quality:  "medium",
	})

	req := httptest.NewRequest(http.MethodPost, "/desktop/sessions/sess-vnc-auth/stream-ticket", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "usr-desktop-ticket", "admin"))
	rec := httptest.NewRecorder()
	sut.handleDesktopStreamTicket(rec, req, terminal.Session{
		ID:     "sess-vnc-auth",
		Target: "desktop-node-01",
		Mode:   "desktop",
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for desktop stream ticket, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		StreamPath      string `json:"stream_path"`
		AudioStreamPath string `json:"audio_stream_path"`
		VNCPassword     string `json:"vnc_password"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode stream ticket payload: %v", err)
	}
	if strings.TrimSpace(payload.VNCPassword) == "" {
		t.Fatal("expected vnc_password for agent-backed VNC stream")
	}
	if strings.Contains(payload.StreamPath, payload.VNCPassword) {
		t.Fatal("stream_path must not include vnc_password")
	}
	if !strings.Contains(payload.AudioStreamPath, "/desktop/sessions/sess-vnc-auth/audio") {
		t.Fatalf("expected audio_stream_path for agent-backed VNC stream, got %q", payload.AudioStreamPath)
	}
	if strings.Contains(payload.AudioStreamPath, payload.VNCPassword) {
		t.Fatal("audio_stream_path must not include vnc_password")
	}

	opts := sut.getDesktopSessionOptions("sess-vnc-auth")
	if opts.VNCPassword != payload.VNCPassword {
		t.Fatalf("expected stored VNC password to match response password, got %q != %q", opts.VNCPassword, payload.VNCPassword)
	}
}

func TestHandleDesktopSessionActionsRejectCrossActorAccess(t *testing.T) {
	sut := newTestAPIServer(t)
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "desktop-node-authz",
		Type:    "node",
		Name:    "Desktop Node Authz",
		Source:  "manual",
		Status:  "online",
	}); err != nil {
		t.Fatalf("failed to seed asset: %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/desktop/sessions", bytes.NewBufferString(`{"target":"desktop-node-authz"}`))
	createReq = createReq.WithContext(contextWithUserID(createReq.Context(), "actor-a"))
	createRec := httptest.NewRecorder()
	sut.handleDesktopSessions(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating desktop session, got %d: %s", createRec.Code, createRec.Body.String())
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create desktop session: %v", err)
	}
	if strings.TrimSpace(created.ID) == "" {
		t.Fatal("expected desktop session id")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/desktop/sessions/"+created.ID, nil)
	getReq = getReq.WithContext(contextWithUserID(getReq.Context(), "actor-b"))
	getRec := httptest.NewRecorder()
	sut.handleDesktopSessionActions(getRec, getReq)

	if getRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-actor desktop session access, got %d", getRec.Code)
	}
}

func TestHandleDesktopSessionActionsRejectNonDesktopSessionID(t *testing.T) {
	sut := newTestAPIServer(t)
	terminalSessionID := mustCreateSession(t, sut)

	req := httptest.NewRequest(http.MethodGet, "/desktop/sessions/"+terminalSessionID, nil)
	rec := httptest.NewRecorder()
	sut.handleDesktopSessionActions(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-desktop session ID, got %d", rec.Code)
	}
}

func TestHandleDesktopSessionActionsDeleteCleansUpSessionState(t *testing.T) {
	sut := newTestAPIServer(t)
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "desktop-node-cleanup",
		Type:    "node",
		Name:    "Desktop Node Cleanup",
		Source:  "manual",
		Status:  "online",
	}); err != nil {
		t.Fatalf("failed to seed asset: %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/desktop/sessions", bytes.NewBufferString(`{"target":"desktop-node-cleanup","protocol":"vnc"}`))
	createReq = createReq.WithContext(contextWithUserID(createReq.Context(), "actor-a"))
	createRec := httptest.NewRecorder()
	sut.handleDesktopSessions(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating desktop session, got %d: %s", createRec.Code, createRec.Body.String())
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create desktop session: %v", err)
	}
	if strings.TrimSpace(created.ID) == "" {
		t.Fatal("expected desktop session id")
	}

	sut.setDesktopSPICEProxyTarget(created.ID, desktopSPICEProxyTarget{
		Host:     "spice.local",
		TLSPort:  61000,
		Password: "secret",
	})
	desktopBridge := &desktopBridge{
		OutputCh:  make(chan []byte, 1),
		AudioCh:   make(chan desktopAudioOutbound, 1),
		ClosedCh:  make(chan struct{}),
		SessionID: created.ID,
		Target:    "desktop-node-cleanup",
	}
	webrtcBridge := &webrtcSignalingBridge{
		ExpectedAgentID: "desktop-node-cleanup",
		ClosedCh:        make(chan struct{}),
	}
	sut.desktopBridges.Store(created.ID, desktopBridge)
	sut.webrtcBridges.Store(created.ID, webrtcBridge)

	deleteReq := httptest.NewRequest(http.MethodDelete, "/desktop/sessions/"+created.ID, nil)
	deleteReq = deleteReq.WithContext(contextWithUserID(deleteReq.Context(), "actor-a"))
	deleteRec := httptest.NewRecorder()
	sut.handleDesktopSessionActions(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 deleting desktop session, got %d: %s", deleteRec.Code, deleteRec.Body.String())
	}
	if _, ok, err := sut.terminalStore.GetSession(created.ID); err != nil {
		t.Fatalf("get session after delete: %v", err)
	} else if ok {
		t.Fatal("expected desktop session to be removed from store")
	}
	if opts := sut.getDesktopSessionOptions(created.ID); opts != (desktopSessionOptions{}) {
		t.Fatalf("expected desktop session options to be cleared, got %+v", opts)
	}
	if _, ok := sut.takeDesktopSPICEProxyTarget(created.ID); ok {
		t.Fatal("expected desktop SPICE proxy target to be cleared")
	}
	if _, ok := sut.desktopBridges.Load(created.ID); ok {
		t.Fatal("expected desktop bridge to be removed")
	}
	if _, ok := sut.webrtcBridges.Load(created.ID); ok {
		t.Fatal("expected webrtc bridge to be removed")
	}
	select {
	case <-desktopBridge.ClosedCh:
	default:
		t.Fatal("expected desktop bridge to be closed")
	}
	select {
	case <-webrtcBridge.ClosedCh:
	default:
		t.Fatal("expected webrtc bridge to be closed")
	}
}

func TestHandleDesktopStreamKeepsSessionOptionsForReconnect(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.setDesktopSessionOptions("sess-reconnect", desktopSessionOptions{
		Protocol: "vnc",
		Quality:  "high",
		Display:  "Display 2",
		Record:   true,
	})

	req := httptest.NewRequest(http.MethodGet, "/desktop/sessions/sess-reconnect/stream", nil)
	rec := httptest.NewRecorder()
	sut.handleDesktopStream(rec, req, terminal.Session{
		ID:     "sess-reconnect",
		Target: "desktop-node-01",
		Mode:   "desktop",
	})

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected direct proxy disabled response, got %d: %s", rec.Code, rec.Body.String())
	}

	opts := sut.getDesktopSessionOptions("sess-reconnect")
	if opts.Protocol != "vnc" || opts.Quality != "high" || opts.Display != "Display 2" || !opts.Record {
		t.Fatalf("expected session options to survive failed stream attempt, got %+v", opts)
	}
}
