package agents

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentcaps"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

func monitoringPeer(t *testing.T) (*testutil.AgentPeer, *Deps) {
	peer := testutil.NewAgentPeer(t, "node01")
	agentcaps.Record(peer.Conn, map[string]string{agentcaps.MetaMode: agentcaps.ModeExplicit, "cap_disks": "list"})
	return peer, &Deps{
		AgentMgr:         peer.Manager,
		PendingAgentCmds: &sync.Map{},
		CurrentHubIdentity: func() *shared.HubSSHIdentity {
			return &shared.HubSSHIdentity{PublicKey: "ssh-ed25519 AAAAexample labtether-hub"}
		},
	}
}

// Fault: commands, updates, agent settings and the Hub SSH key are sent to
// an explicit agent that never advertised them; the job waits for a timeout
// and then reports a vague failure.
func TestAgentRequestsSkipExplicitAgentWithoutSending(t *testing.T) {
	peer, d := monitoringPeer(t)
	result := d.ExecuteViaAgent(terminal.CommandJob{JobID: "job-1", Target: "node01", Command: "uptime"})
	if result.Status != "failed" || result.Output != servicehttp.AgentUnsupportedMessage {
		t.Fatalf("command result %+v, want failed with the fixed words", result)
	}
	for _, mode := range []string{"self", "os_packages"} {
		update := d.ExecuteUpdateViaAgent("upd-"+mode, "node01", mode, nil, time.Second, false)
		if update.Status != "failed" || update.Output != servicehttp.AgentUnsupportedMessage {
			t.Fatalf("update %s result %+v, want failed with the fixed words", mode, update)
		}
	}
	d.PushAgentSettingsApply("node01", map[string]string{"collect_interval": "30"})
	d.SendSSHKeyInstall(peer.Conn)
	if err := d.SendSSHKeyRemove(peer.Conn); !errors.Is(err, agentcaps.ErrUnsupported) {
		t.Fatalf("SSH key removal error %v, want ErrUnsupported", err)
	}
	peer.Silent(t, 150*time.Millisecond)
}

// Fault: the gate also stops an agent without the signal.
func TestAgentRequestsStillReachAgentWithoutTheSignal(t *testing.T) {
	peer := testutil.NewAgentPeer(t, "node01")
	d := &Deps{AgentMgr: peer.Manager, PendingAgentCmds: &sync.Map{}}
	go d.ExecuteViaAgent(terminal.CommandJob{JobID: "job-1", Target: "node01", Command: "uptime", TimeoutSec: 1})
	if got := peer.Next(t); got.Type != agentmgr.MsgCommandRequest {
		t.Fatalf("sent %s, want %s", got.Type, agentmgr.MsgCommandRequest)
	}
}

// Fault: the Hub drops the heartbeat capabilities list of an explicit agent,
// so a Rust agent that advertises "terminal" loses it after a reconnect, or
// it stores the list for a Go agent and changes what that device records.
func TestHeartbeatKeepsTheCapabilitiesListOnlyForExplicitAgents(t *testing.T) {
	for name, tc := range map[string]struct {
		metadata map[string]string
		want     string
		stored   bool
	}{
		"explicit": {map[string]string{agentcaps.MetaMode: agentcaps.ModeExplicit, "cap_network": "list"}, "files,terminal", true},
		"go agent": {map[string]string{"cap_network": "list"}, "", false},
	} {
		t.Run(name, func(t *testing.T) {
			conn := agentmgr.NewAgentConn(nil, "node01", "linux")
			var got assets.HeartbeatRequest
			d := &Deps{
				ProcessHeartbeatRequest: func(req assets.HeartbeatRequest) (*assets.Asset, error) {
					got = req
					return &assets.Asset{ID: req.AssetID}, nil
				},
				AutoProvisionDockerCollectorIfNeeded: func(string, []agentmgr.ConnectorInfo) {},
			}
			data, _ := json.Marshal(agentmgr.HeartbeatData{
				Type: "host", Name: "node01", Source: "agent", Platform: "linux",
				Metadata: tc.metadata, Capabilities: []string{"Terminal", "files", "terminal"},
			})
			d.ProcessAgentHeartbeatInLifecycle(conn, agentmgr.Message{Type: agentmgr.MsgHeartbeat, Data: data})
			value, stored := got.Metadata[agentcaps.MetaCapabilities]
			if stored != tc.stored || value != tc.want {
				t.Fatalf("stored %q (%v), want %q (%v)", value, stored, tc.want, tc.stored)
			}
			explicit := strings.Contains(name, "explicit")
			if agentcaps.ConnOffers(conn, agentmgr.MsgProcessList) == explicit {
				t.Fatalf("connection did not follow the heartbeat (explicit=%v)", explicit)
			}
		})
	}
}
