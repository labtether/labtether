package terminal

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

var ErrPersistentTerminalCleanupUnavailable = errors.New("persistent terminal cleanup path unavailable")

var PersistentTmuxCleanupAgentFunc = func(d *Deps, target, tmuxSessionName string) error {
	return d.KillPersistentTerminalTmuxViaAgent(target, tmuxSessionName)
}

var PersistentTmuxCleanupSSHFunc = func(d *Deps, cfg *terminal.SSHConfig, tmuxSessionName string) error {
	return d.KillPersistentTerminalTmuxViaSSH(cfg, tmuxSessionName)
}

func (d *Deps) TerminatePersistentTerminalRuntime(persistent terminal.PersistentSession) error {
	tmuxSessionName := strings.TrimSpace(persistent.TmuxSessionName)
	if tmuxSessionName == "" {
		return nil
	}

	target := strings.TrimSpace(persistent.Target)
	if target == "" {
		return fmt.Errorf("%w: missing target", ErrPersistentTerminalCleanupUnavailable)
	}

	if d.AgentMgr != nil && d.AgentMgr.IsConnected(target) {
		if err := PersistentTmuxCleanupAgentFunc(d, target, tmuxSessionName); err != nil {
			return fmt.Errorf("failed to end saved shell on connected agent: %w", err)
		}
		return nil
	}

	resolvedSSHConfig, err := d.ResolveSessionSSHConfig(terminal.Session{Target: target})
	if err != nil {
		return fmt.Errorf("failed to resolve ssh config for saved shell cleanup: %w", err)
	}
	if resolvedSSHConfig == nil {
		return fmt.Errorf("%w: connect the agent or configure ssh access before deleting this saved shell", ErrPersistentTerminalCleanupUnavailable)
	}
	if err := PersistentTmuxCleanupSSHFunc(d, resolvedSSHConfig, tmuxSessionName); err != nil {
		return fmt.Errorf("failed to end saved shell over ssh: %w", err)
	}
	return nil
}

func (d *Deps) KillPersistentTerminalTmuxViaAgent(target, tmuxSessionName string) error {
	if d.AgentMgr == nil {
		return errors.New("agent manager unavailable")
	}
	conn, ok := d.AgentMgr.Get(target)
	if !ok {
		return errors.New("agent not connected")
	}

	timeout := shared.EnvOrDefaultDuration("TERMINAL_COMMAND_TIMEOUT", 30*time.Second)
	jobID := shared.GenerateRequestID()
	sessionID := shared.GenerateRequestID()
	commandID := "persistent.tmux.kill"

	payload, err := json.Marshal(agentmgr.TerminalTmuxKillData{
		JobID:       jobID,
		SessionID:   sessionID,
		CommandID:   commandID,
		TmuxSession: strings.TrimSpace(tmuxSessionName),
		Timeout:     int(timeout.Seconds()),
	})
	if err != nil {
		return fmt.Errorf("marshal tmux kill request: %w", err)
	}

	resultCh := make(chan agentmgr.CommandResultData, 1)
	d.PendingAgentCmds.Store(jobID, shared.PendingAgentCommand{
		ResultCh:          resultCh,
		ExpectedAssetID:   target,
		ExpectedSessionID: sessionID,
		ExpectedCommandID: commandID,
	})
	defer d.PendingAgentCmds.Delete(jobID)

	if err := conn.Send(agentmgr.Message{
		Type: agentmgr.MsgTerminalTmuxKill,
		ID:   jobID,
		Data: payload,
	}); err != nil {
		return fmt.Errorf("send tmux kill request to agent: %w", err)
	}

	timer := time.NewTimer(timeout + 5*time.Second)
	defer timer.Stop()

	var result terminal.CommandResult
	select {
	case data := <-resultCh:
		result = terminal.CommandResult{
			JobID:       data.JobID,
			SessionID:   data.SessionID,
			CommandID:   data.CommandID,
			Status:      data.Status,
			Output:      data.Output,
			CompletedAt: time.Now().UTC(),
		}
	case <-timer.C:
		result = terminal.CommandResult{
			JobID:       jobID,
			SessionID:   sessionID,
			CommandID:   commandID,
			Status:      "failed",
			Output:      "agent tmux cleanup timed out waiting for response",
			CompletedAt: time.Now().UTC(),
		}
	}
	if strings.EqualFold(strings.TrimSpace(result.Status), "succeeded") {
		return nil
	}
	output := strings.TrimSpace(result.Output)
	if output == "" {
		output = "agent command failed"
	}
	return errors.New(output)
}

func (d *Deps) KillPersistentTerminalTmuxViaSSH(cfg *terminal.SSHConfig, tmuxSessionName string) error {
	if cfg == nil {
		return fmt.Errorf("%w: ssh config is required", ErrPersistentTerminalCleanupUnavailable)
	}
	if d.ExecuteSSHCommandFn == nil {
		return fmt.Errorf("%w: ssh command executor not configured", ErrPersistentTerminalCleanupUnavailable)
	}
	_, err := d.ExecuteSSHCommandFn(terminal.CommandJob{
		JobID:       shared.GenerateRequestID(),
		SessionID:   shared.GenerateRequestID(),
		CommandID:   "persistent.tmux.kill",
		Target:      strings.TrimSpace(cfg.Host),
		Command:     PersistentTmuxCleanupCommand(tmuxSessionName),
		Mode:        "ssh",
		SSHConfig:   cfg,
		RequestedAt: time.Now().UTC(),
	}, "ssh", 15*time.Second, 8*1024)
	return err
}

// StartArchiveWorker runs a background loop that periodically archives
// detached persistent sessions that have been idle for more than 7 days.
// It returns when ctx is cancelled.
func (d *Deps) StartArchiveWorker(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.archiveStaleDetachedSessions()
		}
	}
}

func (d *Deps) archiveStaleDetachedSessions() {
	if d.TerminalPersistentStore == nil {
		return
	}
	threshold := time.Now().Add(-7 * 24 * time.Hour) // system default: 7 days
	sessions, err := d.TerminalPersistentStore.ListDetachedOlderThan(threshold)
	if err != nil {
		log.Printf("terminal-archive: list_detached_failed err=%v", err)
		return
	}
	for _, ps := range sessions {
		// Respect per-session override
		if ps.ArchiveAfterDays != nil {
			sessionThreshold := time.Now().Add(-time.Duration(*ps.ArchiveAfterDays) * 24 * time.Hour)
			if ps.LastDetachedAt != nil && ps.LastDetachedAt.After(sessionThreshold) {
				continue // not yet past per-session threshold
			}
		}
		if cleanupErr := d.TerminatePersistentTerminalRuntime(ps); cleanupErr != nil {
			log.Printf("terminal-archive: runtime_cleanup_failed id=%s err=%v", ps.ID, cleanupErr)
		}
		if _, archiveErr := d.TerminalPersistentStore.MarkPersistentSessionArchived(ps.ID, time.Now().UTC()); archiveErr != nil {
			log.Printf("terminal-archive: mark_archived_failed id=%s err=%v", ps.ID, archiveErr)
		} else {
			log.Printf("terminal-archive: archived id=%s target=%s", ps.ID, ps.Target)
		}
	}
}

func PersistentTmuxCleanupCommand(tmuxSessionName string) string {
	quotedName := shared.ShellSingleQuote(strings.TrimSpace(tmuxSessionName))
	return "if ! command -v tmux >/dev/null 2>&1; then echo 'tmux not available' >&2; exit 1; fi; " +
		"if tmux has-session -t " + quotedName + " 2>/dev/null; then tmux kill-session -t " + quotedName + "; else exit 0; fi"
}
