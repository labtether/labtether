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
)

// For an agent device whose agent was away, Remote View session create and the
// stream ticket answered 201, so a client that skipped the displays call got a
// session that could never work and only the stream refused. Both must give
// the displays route's answer, and the session must not stay open.
func TestRemoteViewSessionAndTicketSayAgentIsNotConnected(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()
	sut.resetDesktopDepsForTest()
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "desk-agent-01", Type: "node", Name: "Desk", Source: "agent", Status: "online",
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	create := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		sut.handleDesktopSessions(rec, httptest.NewRequest(http.MethodPost, "/desktop/sessions",
			bytes.NewBufferString(`{"target":"desk-agent-01","protocol":"vnc"}`)))
		return rec
	}
	const offline = "agent is not connected (last seen: "

	rec := create()
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), offline) {
		t.Fatalf("create while the agent is away: %d %s", rec.Code, rec.Body.String())
	}
	if sessions, _ := sut.terminalStore.ListSessions(); len(sessions) != 0 {
		t.Fatalf("a refused create left %d sessions", len(sessions))
	}

	// A session made while the agent was there, then the agent left.
	sut.agentMgr.Register(&agentmgr.AgentConn{AssetID: "desk-agent-01"})
	rec = create()
	if rec.Code != http.StatusCreated {
		t.Fatalf("create with the agent connected: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	session, ok, err := sut.terminalStore.GetSession(created.ID)
	if err != nil || !ok {
		t.Fatalf("created session %q not found: %v", created.ID, err)
	}
	sut.agentMgr.Unregister("desk-agent-01")

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/desktop/sessions/"+session.ID+"/stream-ticket", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), session.ActorID, "admin"))
	sut.handleDesktopStreamTicket(rec, req, session)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), offline) {
		t.Fatalf("stream ticket while the agent is away: %d %s", rec.Code, rec.Body.String())
	}
	if _, ok, _ := sut.terminalStore.GetSession(session.ID); ok {
		t.Fatal("the session stayed open after its stream ticket was refused")
	}
}
