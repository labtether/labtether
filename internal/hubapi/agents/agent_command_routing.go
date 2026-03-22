package agents

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/terminal"
)

type PendingAgentCommand = shared.PendingAgentCommand

// executeViaAgent sends a command to a connected agent over WebSocket and
// waits for the result with a timeout.
func (d *Deps) ExecuteViaAgent(cmdJob terminal.CommandJob) terminal.CommandResult {
	conn, ok := d.AgentMgr.Get(cmdJob.Target)
	if !ok {
		return terminal.CommandResult{
			JobID:       cmdJob.JobID,
			SessionID:   cmdJob.SessionID,
			CommandID:   cmdJob.CommandID,
			Status:      "failed",
			Output:      "agent not connected",
			CompletedAt: time.Now().UTC(),
		}
	}

	timeout := shared.EnvOrDefaultDuration("TERMINAL_COMMAND_TIMEOUT", 30*time.Second)
	if cmdJob.TimeoutSec > 0 {
		timeout = time.Duration(cmdJob.TimeoutSec) * time.Second
	}

	req := agentmgr.CommandRequestData{
		JobID:     cmdJob.JobID,
		SessionID: cmdJob.SessionID,
		CommandID: cmdJob.CommandID,
		Command:   cmdJob.Command,
		Timeout:   int(timeout.Seconds()),
	}

	data, err := json.Marshal(req)
	if err != nil {
		return terminal.CommandResult{
			JobID:       cmdJob.JobID,
			SessionID:   cmdJob.SessionID,
			CommandID:   cmdJob.CommandID,
			Status:      "failed",
			Output:      fmt.Sprintf("marshal command request: %v", err),
			CompletedAt: time.Now().UTC(),
		}
	}

	// Set up correlation channel.
	resultCh := make(chan agentmgr.CommandResultData, 1)
	d.PendingAgentCmds.Store(cmdJob.JobID, PendingAgentCommand{
		ResultCh:          resultCh,
		ExpectedAssetID:   cmdJob.Target,
		ExpectedSessionID: cmdJob.SessionID,
		ExpectedCommandID: cmdJob.CommandID,
	})
	defer d.PendingAgentCmds.Delete(cmdJob.JobID)

	if err := conn.Send(agentmgr.Message{
		Type: agentmgr.MsgCommandRequest,
		ID:   cmdJob.JobID,
		Data: data,
	}); err != nil {
		return terminal.CommandResult{
			JobID:       cmdJob.JobID,
			SessionID:   cmdJob.SessionID,
			CommandID:   cmdJob.CommandID,
			Status:      "failed",
			Output:      fmt.Sprintf("send to agent: %v", err),
			CompletedAt: time.Now().UTC(),
		}
	}

	log.Printf("agentws: command %s sent to %s via WebSocket", cmdJob.CommandID, cmdJob.Target)

	// Wait for result with timeout.
	timer := time.NewTimer(timeout + 5*time.Second) // extra buffer beyond agent-side timeout
	defer timer.Stop()

	select {
	case result := <-resultCh:
		return terminal.CommandResult{
			JobID:       result.JobID,
			SessionID:   result.SessionID,
			CommandID:   result.CommandID,
			Status:      result.Status,
			Output:      result.Output,
			CompletedAt: time.Now().UTC(),
		}
	case <-timer.C:
		return terminal.CommandResult{
			JobID:       cmdJob.JobID,
			SessionID:   cmdJob.SessionID,
			CommandID:   cmdJob.CommandID,
			Status:      "failed",
			Output:      "agent command timed out waiting for response",
			CompletedAt: time.Now().UTC(),
		}
	}
}

// executeUpdateViaAgent sends an update.request to a connected agent and waits
// for update.result over the shared pending command correlation map.
func (d *Deps) ExecuteUpdateViaAgent(
	jobID string,
	target string,
	mode string,
	packages []string,
	timeout time.Duration,
	force bool,
) agentmgr.CommandResultData {
	failed := func(message string) agentmgr.CommandResultData {
		return agentmgr.CommandResultData{
			JobID:  jobID,
			Status: "failed",
			Output: message,
		}
	}

	if strings.TrimSpace(jobID) == "" {
		return failed("update request id is required")
	}
	if d.AgentMgr == nil {
		return failed("agent manager unavailable")
	}

	conn, ok := d.AgentMgr.Get(target)
	if !ok {
		return failed("agent not connected")
	}

	if timeout <= 0 {
		timeout = shared.EnvOrDefaultDuration("UPDATE_COMMAND_TIMEOUT", 10*time.Minute)
	}

	req := agentmgr.UpdateRequestData{
		JobID:    jobID,
		Mode:     strings.TrimSpace(mode),
		Force:    force,
		Packages: append([]string(nil), packages...),
	}
	data, err := json.Marshal(req)
	if err != nil {
		return failed(fmt.Sprintf("marshal update request: %v", err))
	}

	resultCh := make(chan agentmgr.CommandResultData, 1)
	d.PendingAgentCmds.Store(jobID, PendingAgentCommand{
		ResultCh:        resultCh,
		ExpectedAssetID: target,
	})
	defer d.PendingAgentCmds.Delete(jobID)

	if err := conn.Send(agentmgr.Message{
		Type: agentmgr.MsgUpdateRequest,
		ID:   jobID,
		Data: data,
	}); err != nil {
		return failed(fmt.Sprintf("send update request to agent: %v", err))
	}

	log.Printf("agentws: update request %s sent to %s (mode=%s force=%v)", jobID, target, req.Mode, req.Force) // #nosec G706 -- Job, target, and mode values are bounded hub-controlled runtime identifiers.

	timer := time.NewTimer(timeout + 15*time.Second)
	defer timer.Stop()

	select {
	case result := <-resultCh:
		if strings.TrimSpace(result.JobID) == "" {
			result.JobID = jobID
		}
		if strings.TrimSpace(result.Status) == "" {
			result.Status = "failed"
		}
		return result
	case <-timer.C:
		return failed("agent update timed out waiting for response")
	}
}
