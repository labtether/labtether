package resources

import (
	"errors"

	"github.com/labtether/labtether/internal/agentcaps"
	"github.com/labtether/labtether/internal/agentmgr"
)

// Errors from RunServiceAction when the agent gave no answer of its own.
var (
	ErrServiceAgentNotConnected = errors.New("agent is not connected")
	ErrServiceAgentTimeout      = errors.New("agent did not respond in time")
	ErrServiceAgentBadAnswer    = errors.New("invalid agent response")
	// ErrServiceAgentUnsupported: the agent did not advertise service
	// actions, so nothing was sent.
	ErrServiceAgentUnsupported = agentcaps.ErrUnsupported
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
	msg, err := d.requestAgent(d.ServiceReplies, agentConn, agentmgr.MsgServiceAction, requestID,
		agentmgr.ServiceActionData{RequestID: requestID, Service: service, Action: action}, assetID, serviceActionTimeout)
	switch {
	case errors.Is(err, agentcaps.ErrUnsupported):
		return agentmgr.ServiceResultData{}, ErrServiceAgentUnsupported
	case agentmgr.RequestKind(err) == agentmgr.RequestTimedOut:
		return agentmgr.ServiceResultData{}, ErrServiceAgentTimeout
	case err != nil:
		return agentmgr.ServiceResultData{}, ErrServiceAgentNotConnected
	}
	result, err := agentmgr.Decode[agentmgr.ServiceResultData](msg)
	if err != nil {
		return agentmgr.ServiceResultData{}, ErrServiceAgentBadAnswer
	}
	return result, nil
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
