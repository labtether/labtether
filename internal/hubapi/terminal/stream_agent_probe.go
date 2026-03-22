package terminal

import (
	"encoding/json"
	"log"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
)

// ProbeAgentTmux returns cached tmux capability metadata when available.
// If unknown, it triggers an async probe and returns immediately (no connect-path wait).
func (d *Deps) ProbeAgentTmux(agentConn *agentmgr.AgentConn) agentmgr.TerminalProbeResponse {
	if agentConn == nil {
		return agentmgr.TerminalProbeResponse{}
	}

	hasTmux := strings.TrimSpace(agentConn.Meta("terminal.tmux.has"))
	tmuxPath := strings.TrimSpace(agentConn.Meta("terminal.tmux.path"))
	switch hasTmux {
	case "true":
		return agentmgr.TerminalProbeResponse{HasTmux: true, TmuxPath: tmuxPath}
	case "false":
		return agentmgr.TerminalProbeResponse{HasTmux: false, TmuxPath: tmuxPath}
	}

	d.StartAgentTmuxProbeAsync(agentConn)

	return agentmgr.TerminalProbeResponse{}
}

func (d *Deps) StartAgentTmuxProbeAsync(agentConn *agentmgr.AgentConn) bool {
	if agentConn == nil {
		return false
	}

	// Avoid flooding probes while one is already in flight for this connection.
	if strings.TrimSpace(agentConn.Meta("terminal.tmux.probe_pending")) == "true" {
		return false
	}
	agentConn.SetMeta("terminal.tmux.probe_pending", "true")

	assetID := strings.TrimSpace(agentConn.AssetID)
	go func(conn *agentmgr.AgentConn) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("terminal: async tmux probe panic for %s: %v", assetID, recovered)
				conn.SetMeta("terminal.tmux.probe_pending", "false")
			}
		}()
		if err := conn.Send(agentmgr.Message{Type: agentmgr.MsgTerminalProbe}); err != nil {
			log.Printf("terminal: failed to send async tmux probe to %s: %v", assetID, err)
			conn.SetMeta("terminal.tmux.probe_pending", "false")
			return
		}
		log.Printf("terminal: requested tmux capability probe for %s", assetID)
	}(agentConn)

	return true
}

// ProcessAgentTerminalProbed handles terminal.probed from agent.
func (d *Deps) ProcessAgentTerminalProbed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var resp agentmgr.TerminalProbeResponse
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		log.Printf("terminal: failed to unmarshal tmux probe response: %v", err)
		return
	}
	if conn != nil {
		log.Printf("terminal: tmux probe result for %s: has_tmux=%t path=%q", conn.AssetID, resp.HasTmux, resp.TmuxPath)
		conn.SetMeta("terminal.tmux.has", strconv.FormatBool(resp.HasTmux))
		conn.SetMeta("terminal.tmux.path", strings.TrimSpace(resp.TmuxPath))
		conn.SetMeta("terminal.tmux.probe_pending", "false")
	} else {
		return
	}

	probeKey := "probe:" + conn.AssetID
	if ch, ok := d.TerminalBridges.Load(probeKey); ok {
		if probeCh, ok := ch.(chan agentmgr.TerminalProbeResponse); ok {
			select {
			case probeCh <- resp:
			default:
			}
		}
	}
}
