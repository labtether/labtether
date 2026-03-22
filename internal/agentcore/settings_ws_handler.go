package agentcore

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

// handleConfigUpdate applies runtime configuration changes from the hub.
func handleConfigUpdate(transport *wsTransport, msg agentmgr.Message, runtime *Runtime) {
	var data agentmgr.ConfigUpdateData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid config update: %v", err)
		return
	}

	log.Printf("agentws: received config update: collect=%v heartbeat=%v loglevel=%v",
		data.CollectIntervalSec, data.HeartbeatIntervalSec, data.LogLevel)

	if runtime == nil {
		return
	}

	changed := false
	if data.CollectIntervalSec != nil {
		v := *data.CollectIntervalSec
		switch {
		case v == 0:
			runtime.collectIntervalOverride.Store(0)
			runtime.mu.Lock()
			runtime.cfg.CollectInterval = runtime.baseCollectInterval
			runtime.mu.Unlock()
			changed = true
		case v >= 2 && v <= 300:
			runtime.collectIntervalOverride.Store(int64(v))
			runtime.mu.Lock()
			runtime.cfg.CollectInterval = time.Duration(v) * time.Second
			runtime.mu.Unlock()
			changed = true
		default:
			log.Printf("agentws: ignoring invalid collect interval %d (must be 2-300)", v)
		}
	}
	if data.HeartbeatIntervalSec != nil {
		v := *data.HeartbeatIntervalSec
		switch {
		case v == 0:
			runtime.heartbeatIntervalOverride.Store(0)
			runtime.mu.Lock()
			runtime.cfg.HeartbeatInterval = runtime.baseHeartbeatInterval
			runtime.mu.Unlock()
			changed = true
		case v >= 5 && v <= 600:
			runtime.heartbeatIntervalOverride.Store(int64(v))
			runtime.mu.Lock()
			runtime.cfg.HeartbeatInterval = time.Duration(v) * time.Second
			runtime.mu.Unlock()
			changed = true
		default:
			log.Printf("agentws: ignoring invalid heartbeat interval %d (must be 5-600)", v)
		}
	}

	if changed {
		// Persist applied config to disk.
		persistAppliedConfig(runtime)
	}

	// Send acknowledgment back to hub.
	ackData, _ := json.Marshal(agentmgr.ConfigAppliedData{
		CollectIntervalSec:   runtime.effectiveCollectIntervalSec(),
		HeartbeatIntervalSec: runtime.effectiveHeartbeatIntervalSec(),
	})
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgConfigApplied,
		Data: ackData,
	})
}

func handleAgentSettingsApply(transport *wsTransport, msg agentmgr.Message, runtime *Runtime) {
	var req agentmgr.AgentSettingsApplyData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("agentws: invalid agent.settings.apply: %v", err)
		return
	}

	response := agentmgr.AgentSettingsAppliedData{
		RequestID: req.RequestID,
		Revision:  req.Revision,
		Applied:   false,
	}

	if runtime == nil {
		response.Error = "runtime unavailable"
		sendAgentSettingsApplied(transport, runtime, response)
		return
	}

	if !runtime.allowRemoteOverrides() {
		response.Error = "remote overrides are disabled on this agent"
		sendAgentSettingsApplied(transport, runtime, response)
		sendAgentSettingsState(transport, runtime, req.Revision)
		return
	}

	if runtime.deviceIdentity != nil && strings.TrimSpace(req.ExpectedFingerprint) != "" &&
		!strings.EqualFold(strings.TrimSpace(req.ExpectedFingerprint), runtime.deviceIdentity.Fingerprint) {
		response.Error = "fingerprint mismatch for settings apply"
		sendAgentSettingsApplied(transport, runtime, response)
		sendAgentSettingsState(transport, runtime, req.Revision)
		return
	}

	applied, restartRequired, err := runtime.applyAgentSettings(req.Values)
	if err != nil {
		response.Error = err.Error()
		sendAgentSettingsApplied(transport, runtime, response)
		sendAgentSettingsState(transport, runtime, req.Revision)
		return
	}

	response.Applied = true
	response.RestartRequired = restartRequired
	response.AppliedValues = applied
	response.AppliedAt = time.Now().UTC().Format(time.RFC3339)
	sendAgentSettingsApplied(transport, runtime, response)
	sendAgentSettingsState(transport, runtime, req.Revision)
}

func sendAgentSettingsApplied(transport *wsTransport, runtime *Runtime, response agentmgr.AgentSettingsAppliedData) {
	if runtime != nil && runtime.deviceIdentity != nil {
		response.Fingerprint = runtime.deviceIdentity.Fingerprint
	}
	data, err := json.Marshal(response)
	if err != nil {
		log.Printf("agentws: failed to marshal agent.settings.applied: %v", err)
		return
	}
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgAgentSettingsApplied,
		Data: data,
	})
}
