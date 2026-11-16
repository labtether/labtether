package desktop_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
)

// For an agent device whose agent was away, Remote View session create and the
// stream ticket answered 201, so a client that skipped the displays call got a
// session that could never work and only the stream refused. Both must give
// the displays route's answer, and the session must not stay open.
func TestRemoteViewSessionAndTicketSayAgentIsNotConnected(t *testing.T) {
	sut := hubdeps.New(t)
	sut.AgentMgr = agentmgr.NewManager()
	sut.ResetDesktopDeps()
	if _, err := sut.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "desk-agent-01", Type: "node", Name: "Desk", Source: "agent", Status: "online",
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	create := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		sut.DesktopDeps().HandleDesktopSessions(rec, httptest.NewRequest(http.MethodPost, "/desktop/sessions",
			bytes.NewBufferString(`{"target":"desk-agent-01","protocol":"vnc"}`)))
		return rec
	}
	const offline = "agent is not connected (last seen: "

	rec := create()
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), offline) {
		t.Fatalf("create while the agent is away: %d %s", rec.Code, rec.Body.String())
	}
	if sessions, _ := sut.TerminalStore.ListSessions(); len(sessions) != 0 {
		t.Fatalf("a refused create left %d sessions", len(sessions))
	}

	// A session made while the agent was there, then the agent left.
	sut.AgentMgr.Register(&agentmgr.AgentConn{AssetID: "desk-agent-01"})
	rec = create()
	if rec.Code != http.StatusCreated {
		t.Fatalf("create with the agent connected: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	session, ok, err := sut.TerminalStore.GetSession(created.ID)
	if err != nil || !ok {
		t.Fatalf("created session %q not found: %v", created.ID, err)
	}
	sut.AgentMgr.Unregister("desk-agent-01")

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/desktop/sessions/"+session.ID+"/stream-ticket", nil)
	req = req.WithContext(apiv2.ContextWithPrincipal(req.Context(), session.ActorID, "admin"))
	sut.DesktopDeps().HandleDesktopStreamTicket(rec, req, session)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), offline) {
		t.Fatalf("stream ticket while the agent is away: %d %s", rec.Code, rec.Body.String())
	}
	if _, ok, _ := sut.TerminalStore.GetSession(session.ID); ok {
		t.Fatal("the session stayed open after its stream ticket was refused")
	}
}
