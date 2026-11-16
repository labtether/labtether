package dockerpkg

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentcaps"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/terminal"
)

// Fault: a container shell opens on an explicit agent that never advertised
// Docker, and the exec session never starts.
func TestDockerExecRefusesExplicitAgentWithoutDocker(t *testing.T) {
	peer := testutil.NewAgentPeer(t, "node01")
	agentcaps.Record(peer.Conn, map[string]string{agentcaps.MetaMode: agentcaps.ModeExplicit, "cap_disks": "list"})
	d := &Deps{AgentMgr: peer.Manager}
	rec := httptest.NewRecorder()
	d.HandleDockerExecTerminalStream(rec, httptest.NewRequest(http.MethodGet, "/terminal/stream", nil), terminal.Session{ID: "s1"}, "node01", "abc123")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got %d %s, want 422", rec.Code, rec.Body.String())
	}
	peer.Silent(t, 150*time.Millisecond)
}
