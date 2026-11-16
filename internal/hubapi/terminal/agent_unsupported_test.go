package terminal

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentcaps"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

func explicitWithoutTerminal(t *testing.T) (*testutil.AgentPeer, *Deps) {
	peer := testutil.NewAgentPeer(t, "node01")
	agentcaps.Record(peer.Conn, map[string]string{agentcaps.MetaMode: agentcaps.ModeExplicit, "cap_network": "list"})
	return peer, &Deps{AgentMgr: peer.Manager, PendingAgentCmds: &sync.Map{}}
}

// Fault: the Terminal tab opens a stream to an explicit agent that never
// advertised a terminal, and the shell never starts.
func TestTerminalStreamRefusesExplicitAgentWithoutSending(t *testing.T) {
	peer, d := explicitWithoutTerminal(t)
	rec := httptest.NewRecorder()
	d.HandleAgentTerminalStream(rec, httptest.NewRequest(http.MethodGet, "/terminal/stream", nil), terminal.Session{ID: "s1", Target: "node01"})
	var body struct{ Error string }
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusUnprocessableEntity || body.Error != servicehttp.AgentUnsupportedMessage {
		t.Fatalf("got %d %s, want 422", rec.Code, rec.Body.String())
	}
	peer.Silent(t, 150*time.Millisecond)
}

// Fault: background work (the tmux probe on connect, persistent session
// cleanup) sends terminal requests to an explicit agent without a terminal.
func TestTerminalBackgroundWorkSkipsExplicitAgent(t *testing.T) {
	peer, d := explicitWithoutTerminal(t)
	if d.StartAgentTmuxProbeAsync(peer.Conn) {
		t.Fatal("tmux probe started")
	}
	if err := d.KillPersistentTerminalTmuxViaAgent("node01", "lt-s1"); !errors.Is(err, agentcaps.ErrUnsupported) {
		t.Fatalf("tmux kill error %v, want ErrUnsupported", err)
	}
	peer.Silent(t, 150*time.Millisecond)

	legacy := testutil.NewAgentPeer(t, "node02")
	if !(&Deps{AgentMgr: legacy.Manager}).StartAgentTmuxProbeAsync(legacy.Conn) {
		t.Fatal("tmux probe skipped for an agent without the signal")
	}
	if got := legacy.Next(t); got.Type != agentmgr.MsgTerminalProbe {
		t.Fatalf("sent %s, want %s", got.Type, agentmgr.MsgTerminalProbe)
	}
}
