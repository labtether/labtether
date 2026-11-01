package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/terminal"
)

type PendingAgentCommand = shared.PendingAgentCommand

const maxAgentCommandTimeout = 5 * time.Minute

// Logical reservations never accept wire replies. Context attempts use fresh
// string keys so a late result cannot complete a retry of the same queue job.
type agentLogicalRequestKey string

// executeViaAgent sends a command to a connected agent over WebSocket and
// waits for the result with a timeout.
func (d *Deps) ExecuteViaAgent(cmdJob terminal.CommandJob) terminal.CommandResult {
	return d.executeViaAgent(context.Background(), cmdJob, false)
}

func (d *Deps) ExecuteViaAgentContext(ctx context.Context, cmdJob terminal.CommandJob) terminal.CommandResult {
	return d.executeViaAgent(ctx, cmdJob, true)
}

func (d *Deps) executeViaAgent(ctx context.Context, cmdJob terminal.CommandJob, privateAttempt bool) terminal.CommandResult {
	canceled := func(sent bool) terminal.CommandResult {
		message := "agent command canceled before dispatch: " + ctx.Err().Error()
		if sent {
			message = "agent command wait canceled; outcome unknown: " + ctx.Err().Error()
		}
		return terminal.CommandResult{JobID: cmdJob.JobID, SessionID: cmdJob.SessionID,
			CommandID: cmdJob.CommandID, Status: "failed", Output: message, CompletedAt: time.Now().UTC()}
	}
	if ctx.Err() != nil {
		return canceled(false)
	}
	wireID := cmdJob.JobID
	if privateAttempt {
		wireID = "command_" + shared.GenerateRequestID()
	}
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

	timeout := agentCommandTimeoutFromSeconds(
		cmdJob.TimeoutSec,
		shared.EnvOrDefaultDuration("TERMINAL_COMMAND_TIMEOUT", 30*time.Second),
	)

	req := agentmgr.CommandRequestData{
		JobID:     wireID,
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
	pending := &PendingAgentCommand{
		ExpectedConn:      conn,
		ResultCh:          resultCh,
		ExpectedAssetID:   cmdJob.Target,
		ExpectedSessionID: cmdJob.SessionID,
		ExpectedCommandID: cmdJob.CommandID,
	}
	logicalKey := agentLogicalRequestKey(cmdJob.JobID)
	if _, loaded := d.PendingAgentCmds.LoadOrStore(logicalKey, pending); loaded {
		return terminal.CommandResult{JobID: cmdJob.JobID, SessionID: cmdJob.SessionID,
			CommandID: cmdJob.CommandID, Status: "failed", Output: "agent request already in progress", CompletedAt: time.Now().UTC()}
	}
	defer d.PendingAgentCmds.CompareAndDelete(logicalKey, pending)
	if _, loaded := d.PendingAgentCmds.LoadOrStore(wireID, pending); loaded {
		return terminal.CommandResult{JobID: cmdJob.JobID, SessionID: cmdJob.SessionID,
			CommandID: cmdJob.CommandID, Status: "failed", Output: "agent request already in progress", CompletedAt: time.Now().UTC()}
	}
	defer d.PendingAgentCmds.CompareAndDelete(wireID, pending)

	if err := conn.SendContext(ctx, agentmgr.Message{
		Type: agentmgr.MsgCommandRequest,
		ID:   wireID,
		Data: data,
	}); err != nil {
		if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
			return canceled(false)
		}
		return terminal.CommandResult{
			JobID:       cmdJob.JobID,
			SessionID:   cmdJob.SessionID,
			CommandID:   cmdJob.CommandID,
			Status:      "failed",
			Output:      fmt.Sprintf("send to agent: %v; outcome unknown", err),
			CompletedAt: time.Now().UTC(),
		}
	}

	log.Printf("agentws: command %s sent to %s via WebSocket", cmdJob.CommandID, cmdJob.Target)

	// Wait for result with timeout.
	timer := time.NewTimer(timeout + 5*time.Second) // extra buffer beyond agent-side timeout
	defer timer.Stop()

	select {
	case result := <-resultCh:
		if ctx.Err() != nil {
			return canceled(true)
		}
		return terminal.CommandResult{
			JobID:       cmdJob.JobID,
			SessionID:   result.SessionID,
			CommandID:   result.CommandID,
			Status:      result.Status,
			Output:      result.Output,
			ExitCode:    result.ExitCode,
			CompletedAt: time.Now().UTC(),
		}
	case <-ctx.Done():
		return canceled(true)
	case <-conn.Done():
		return terminal.CommandResult{JobID: cmdJob.JobID, SessionID: cmdJob.SessionID,
			CommandID: cmdJob.CommandID, Status: "failed", Output: "agent disconnected before command result; outcome unknown", CompletedAt: time.Now().UTC()}
	case <-timer.C:
		return terminal.CommandResult{
			JobID:       cmdJob.JobID,
			SessionID:   cmdJob.SessionID,
			CommandID:   cmdJob.CommandID,
			Status:      "failed",
			Output:      "agent command timed out waiting for response; outcome unknown",
			CompletedAt: time.Now().UTC(),
		}
	}
}

func agentCommandTimeoutFromSeconds(timeoutSec int, fallback time.Duration) time.Duration {
	if fallback <= 0 {
		fallback = 30 * time.Second
	}
	if timeoutSec <= 0 {
		if fallback > maxAgentCommandTimeout {
			return maxAgentCommandTimeout
		}
		return fallback
	}
	maxSeconds := int(maxAgentCommandTimeout / time.Second)
	if timeoutSec > maxSeconds {
		return maxAgentCommandTimeout
	}
	return time.Duration(timeoutSec) * time.Second
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
	return d.executeUpdateViaAgent(context.Background(), jobID, target, mode, packages, timeout, force, false)
}

func (d *Deps) ExecuteUpdateViaAgentContext(ctx context.Context, jobID, target, mode string, packages []string, timeout time.Duration, force bool) agentmgr.CommandResultData {
	return d.executeUpdateViaAgent(ctx, jobID, target, mode, packages, timeout, force, true)
}

func (d *Deps) executeUpdateViaAgent(ctx context.Context, jobID, target, mode string, packages []string, timeout time.Duration, force, privateAttempt bool) agentmgr.CommandResultData {
	failed := func(message string) agentmgr.CommandResultData {
		return agentmgr.CommandResultData{
			JobID:  jobID,
			Status: "failed",
			Output: message,
		}
	}

	if err := ctx.Err(); err != nil {
		return failed("agent update canceled before dispatch: " + err.Error())
	}
	wireID := jobID
	if privateAttempt {
		wireID = "update_" + shared.GenerateRequestID()
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
		JobID:    wireID,
		Mode:     strings.TrimSpace(mode),
		Force:    force,
		Packages: append([]string(nil), packages...),
	}
	data, err := json.Marshal(req)
	if err != nil {
		return failed(fmt.Sprintf("marshal update request: %v", err))
	}

	resultCh := make(chan agentmgr.CommandResultData, 1)
	pending := &PendingAgentCommand{
		ExpectedConn:    conn,
		ResultCh:        resultCh,
		ExpectedAssetID: target,
	}
	logicalKey := agentLogicalRequestKey(jobID)
	if _, loaded := d.PendingAgentCmds.LoadOrStore(logicalKey, pending); loaded {
		return failed("agent request already in progress")
	}
	defer d.PendingAgentCmds.CompareAndDelete(logicalKey, pending)
	if _, loaded := d.PendingAgentCmds.LoadOrStore(wireID, pending); loaded {
		return failed("agent request already in progress")
	}
	defer d.PendingAgentCmds.CompareAndDelete(wireID, pending)
	finishProgress := d.trackAgentUpdate(wireID, conn, timeout+15*time.Second)
	defer finishProgress()

	if err := conn.SendContext(ctx, agentmgr.Message{
		Type: agentmgr.MsgUpdateRequest,
		ID:   wireID,
		Data: data,
	}); err != nil {
		if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
			return failed("agent update canceled before dispatch: " + ctx.Err().Error())
		}
		return failed(fmt.Sprintf("send update request to agent: %v; outcome unknown", err))
	}

	log.Printf("agentws: update request %s sent to %s (mode=%s force=%v)", jobID, target, req.Mode, req.Force) // #nosec G706 -- Job, target, and mode values are bounded hub-controlled runtime identifiers.

	timer := time.NewTimer(timeout + 15*time.Second)
	defer timer.Stop()

	select {
	case result := <-resultCh:
		if err := ctx.Err(); err != nil {
			return failed("agent update wait canceled; outcome unknown: " + err.Error())
		}
		result.JobID = jobID
		if strings.TrimSpace(result.Status) == "" {
			result.Status = "failed"
		}
		return result
	case <-ctx.Done():
		return failed("agent update wait canceled; outcome unknown: " + ctx.Err().Error())
	case <-conn.Done():
		return failed("agent disconnected before update result; outcome unknown")
	case <-timer.C:
		return failed("agent update timed out waiting for response; outcome unknown")
	}
}
