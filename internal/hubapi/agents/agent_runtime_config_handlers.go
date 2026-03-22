package agents

import (
	"encoding/json"
	"log"

	"github.com/labtether/labtether/internal/agentmgr"
)

// sendConfigUpdate sends the current runtime configuration to a connected agent.
func (d *Deps) SendConfigUpdate(conn *agentmgr.AgentConn) {
	if d.RuntimeStore == nil {
		return
	}
	settings, err := d.RuntimeStore.ListRuntimeSettingOverrides()
	if err != nil {
		return
	}

	collectValue := 0
	heartbeatValue := 0
	if v, ok := settings["agent_collect_interval_sec"]; ok {
		if n := ParseIntSafe(v); n > 0 {
			collectValue = n
		}
	}
	if v, ok := settings["agent_heartbeat_interval_sec"]; ok {
		if n := ParseIntSafe(v); n > 0 {
			heartbeatValue = n
		}
	}

	data, _ := json.Marshal(agentmgr.ConfigUpdateData{
		CollectIntervalSec:   &collectValue,
		HeartbeatIntervalSec: &heartbeatValue,
	})
	if err := conn.Send(agentmgr.Message{
		Type: agentmgr.MsgConfigUpdate,
		Data: data,
	}); err != nil {
		log.Printf("agentws: failed to send config.update to %s: %v", conn.AssetID, err)
	}
}

func ParseIntSafe(s string) int {
	if len(s) == 0 || len(s) > 9 {
		return 0 // empty or too large to be a valid interval
	}
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			return 0
		}
	}
	return n
}

// processAgentConfigApplied handles the confirmation that the agent applied config.
func (d *Deps) ProcessAgentConfigApplied(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.ConfigAppliedData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid config.applied from %s: %v", conn.AssetID, err)
		return
	}
	log.Printf("agentws: config applied on %s: collect=%ds heartbeat=%ds",
		conn.AssetID, data.CollectIntervalSec, data.HeartbeatIntervalSec)
}
