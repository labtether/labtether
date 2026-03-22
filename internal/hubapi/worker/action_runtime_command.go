package worker

import (
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/actions"
	opspkg "github.com/labtether/labtether/internal/hubapi/operations"
	"github.com/labtether/labtether/internal/terminal"
)

// ExecuteCommandAction executes a command-type action job.
// If a connected agent is available for the target, the command is routed
// through the agent WebSocket. Otherwise it falls back to the in-process
// executor (simulated / SSH / local depending on environment config).
func (d *Deps) ExecuteCommandAction(job actions.Job) actions.Result {
	target := strings.TrimSpace(job.Target)
	command := strings.TrimSpace(job.Command)
	if target == "" || command == "" {
		return actions.Result{
			JobID:       job.JobID,
			RunID:       job.RunID,
			Status:      actions.StatusFailed,
			Error:       "command actions require target and command",
			Steps:       []actions.StepResult{{Name: "execute_command", Status: actions.StatusFailed, Error: "missing target or command"}},
			CompletedAt: time.Now().UTC(),
		}
	}

	if d.AgentMgr == nil || !d.AgentMgr.IsConnected(target) || d.ExecuteViaAgent == nil {
		// No connected agent for the target; preserve existing simulated
		// fallback behavior in non-agent environments.
		return opspkg.ExecuteActionInProcess(job, nil)
	}

	cmdResult := d.ExecuteViaAgent(terminal.CommandJob{
		JobID:       job.JobID,
		SessionID:   job.RunID,
		CommandID:   fmt.Sprintf("action-%s", job.RunID),
		ActorID:     job.ActorID,
		Target:      target,
		Command:     command,
		Mode:        "structured",
		RequestedAt: job.RequestedAt,
	})

	status := actions.StatusSucceeded
	errMessage := ""
	if !strings.EqualFold(strings.TrimSpace(cmdResult.Status), "succeeded") {
		status = actions.StatusFailed
		errMessage = strings.TrimSpace(cmdResult.Output)
		if errMessage == "" {
			errMessage = "command execution failed"
		}
	}

	step := actions.StepResult{
		Name:   "execute_command",
		Status: status,
		Output: strings.TrimSpace(cmdResult.Output),
	}
	if status == actions.StatusFailed {
		step.Error = errMessage
	}

	return actions.Result{
		JobID:       job.JobID,
		RunID:       job.RunID,
		Status:      status,
		Output:      strings.TrimSpace(cmdResult.Output),
		Error:       errMessage,
		Steps:       []actions.StepResult{step},
		CompletedAt: time.Now().UTC(),
	}
}
