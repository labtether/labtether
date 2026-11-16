package collectors

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/testutil"
)

// The Services page's sync button asks one agent to look for web services
// again. The agent acts only on a webservice.sync message, so its type is the
// whole request; with any other type the button would do nothing.
func TestHandleWebServiceSyncAsksTheChosenAgent(t *testing.T) {
	peer := testutil.NewAgentPeer(t, "nas01")
	d := &Deps{AgentMgr: peer.Manager}
	rec := httptest.NewRecorder()
	d.HandleWebServiceSync(rec, httptest.NewRequest(http.MethodPost, "/api/v1/services/web/sync?host=nas01", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if msg := peer.Next(t); msg.Type != agentmgr.MsgWebServiceSync || msg.ID != "" || msg.Data != nil {
		t.Fatalf("agent got %+v, want a bare webservice.sync", msg)
	}
}
