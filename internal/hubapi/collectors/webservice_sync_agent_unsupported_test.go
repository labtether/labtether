package collectors

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentcaps"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/testutil"
)

// Fault: web service sync is sent to an explicit agent that never advertised
// it, so the request is lost and the caller is told it was queued.
func TestWebServiceSyncSkipsExplicitAgentWithoutIt(t *testing.T) {
	goAgent := testutil.NewAgentPeer(t, "node-go")
	explicit := testutil.NewAgentPeer(t, "node-mon")
	goAgent.Manager.Register(explicit.Conn)
	agentcaps.Record(explicit.Conn, map[string]string{agentcaps.MetaMode: agentcaps.ModeExplicit, "cap_disks": "list"})
	d := &Deps{AgentMgr: goAgent.Manager}

	rec := httptest.NewRecorder()
	d.HandleWebServiceSync(rec, httptest.NewRequest(http.MethodPost, "/api/v1/services/web/sync?host=node-mon", nil))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("one explicit host: got %d %s, want 422", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	d.HandleWebServiceSync(rec, httptest.NewRequest(http.MethodPost, "/api/v1/services/web/sync", nil))
	if rec.Code != http.StatusAccepted || strings.Contains(rec.Body.String(), "node-mon") {
		t.Fatalf("all hosts: got %d %s, want 202 without node-mon", rec.Code, rec.Body.String())
	}
	if got := goAgent.Next(t); got.Type != agentmgr.MsgWebServiceSync {
		t.Fatalf("Go agent got %s, want %s", got.Type, agentmgr.MsgWebServiceSync)
	}
	explicit.Silent(t, 150*time.Millisecond)
}
