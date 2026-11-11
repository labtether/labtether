package resources

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

// Errors from RunServiceAction when the agent gave no answer of its own.
var (
	ErrServiceAgentNotConnected = errors.New("agent is not connected")
	ErrServiceAgentTimeout      = errors.New("agent did not respond in time")
	ErrServiceAgentBadAnswer    = errors.New("invalid agent response")
)

// RunServiceAction asks a device's agent to start, stop, restart, enable or
// disable a service and waits for its answer. The device Services page and the
// bulk service API both use it, so neither depends on the shell command
// allowlist, and both work wherever the agent manages services (systemd,
// Windows services, launchd).
func (d *Deps) RunServiceAction(assetID, service, action string) (agentmgr.ServiceResultData, error) {
	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		return agentmgr.ServiceResultData{}, ErrServiceAgentNotConnected
	}

	requestID := generateRequestID()
	bridge := &ServiceBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: assetID,
	}
	d.ServiceBridges.Store(requestID, bridge)
	defer d.ServiceBridges.Delete(requestID)

	data, _ := json.Marshal(agentmgr.ServiceActionData{
		RequestID: requestID,
		Service:   service,
		Action:    action,
	})
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgServiceAction,
		ID:   requestID,
		Data: data,
	}); err != nil {
		return agentmgr.ServiceResultData{}, ErrServiceAgentNotConnected
	}

	select {
	case msg := <-bridge.Ch:
		var result agentmgr.ServiceResultData
		if err := json.Unmarshal(msg.Data, &result); err != nil {
			return agentmgr.ServiceResultData{}, ErrServiceAgentBadAnswer
		}
		return result, nil
	case <-time.After(serviceActionTimeout):
		return agentmgr.ServiceResultData{}, ErrServiceAgentTimeout
	}
}

// serviceActionWhat finishes the sentence for a device that cannot manage services.
const serviceActionWhat = "services can't be started, stopped or restarted"

// ServiceActionFailureWords says in words why the agent could not act on the
// service: a device without systemd says so, anything else gives the last
// error lines the agent reported.
func ServiceActionFailureWords(result agentmgr.ServiceResultData) string {
	if words, ok := missingCommandWords(result.Error, serviceActionWhat); ok {
		return words
	}
	return agentFailureMessage(result.Error, result.Output)
}
