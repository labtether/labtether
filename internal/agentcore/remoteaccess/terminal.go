package remoteaccess

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/securityruntime"
)

// TerminalSession tracks an active PTY shell session on the agent.
type TerminalSession struct {
	sessionID string
	Ptmx      *os.File
	cmd       *exec.Cmd
	done      chan struct{}
}

const MaxTerminalSessions = 10

// TerminalManager manages PTY sessions on the agent.
type TerminalManager struct {
	Mu       sync.Mutex
	Sessions map[string]*TerminalSession
}

func NewTerminalManager() *TerminalManager {
	return &TerminalManager{
		Sessions: make(map[string]*TerminalSession),
	}
}

// HandleTerminalProbe checks if tmux is available on this agent and reports back.
func (tm *TerminalManager) HandleTerminalProbe(transport MessageSender) {
	tmuxPath, err := exec.LookPath("tmux")
	hasTmux := err == nil && tmuxPath != ""
	resp := agentmgr.TerminalProbeResponse{
		HasTmux:  hasTmux,
		TmuxPath: tmuxPath,
	}
	payload, _ := json.Marshal(resp)
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgTerminalProbed,
		Data: payload,
	})
}

// HandleTerminalStart spawns a new PTY shell and starts streaming output.
func (tm *TerminalManager) HandleTerminalStart(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.TerminalStartData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("terminal: invalid start request: %v", err)
		return
	}

	if req.SessionID == "" {
		log.Printf("terminal: start request missing session_id")
		return
	}

	tm.Mu.Lock()
	if len(tm.Sessions) >= MaxTerminalSessions {
		tm.Mu.Unlock()
		log.Printf("terminal: max sessions (%d) reached", MaxTerminalSessions)
		SendTerminalClosed(transport, req.SessionID, "max terminal sessions reached")
		return
	}
	if _, exists := tm.Sessions[req.SessionID]; exists {
		tm.Mu.Unlock()
		log.Printf("terminal: session %s already exists", req.SessionID)
		return
	}
	tm.Mu.Unlock()

	// Find a shell
	shell := req.Shell
	if shell == "" {
		shell = DetectShell()
	}

	cols := req.Cols
	rows := req.Rows
	if cols <= 0 {
		cols = 120
	}
	if rows <= 0 {
		rows = 40
	}

	// Determine command to run: tmux session or plain shell.
	tmuxAttached := false
	var cmd *exec.Cmd
	var err error
	if req.UseTmux && req.TmuxSession != "" {
		tmuxPath, lookErr := exec.LookPath("tmux")
		if lookErr == nil && tmuxPath != "" {
			// tmux new-session -A -s <name> creates or attaches to an existing session.
			cmd, err = securityruntime.NewCommand(tmuxPath, "new-session", "-A", "-s", req.TmuxSession)
			if err != nil {
				log.Printf("terminal: tmux command blocked by runtime policy for session %s: %v", req.SessionID, err)
				SendTerminalClosed(transport, req.SessionID, "failed to start tmux: "+err.Error())
				return
			}
			tmuxAttached = true
		} else {
			log.Printf("terminal: tmux requested but not found for session %s, falling back to plain shell", req.SessionID)
			cmd, err = securityruntime.NewCommand(shell)
			if err != nil {
				log.Printf("terminal: shell command blocked by runtime policy for session %s: %v", req.SessionID, err)
				SendTerminalClosed(transport, req.SessionID, "failed to start shell: "+err.Error())
				return
			}
		}
	} else {
		cmd, err = securityruntime.NewCommand(shell)
		if err != nil {
			log.Printf("terminal: shell command blocked by runtime policy for session %s: %v", req.SessionID, err)
			SendTerminalClosed(transport, req.SessionID, "failed to start shell: "+err.Error())
			return
		}
	}
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
	if err != nil {
		log.Printf("terminal: failed to start PTY for session %s: %v", req.SessionID, err)
		SendTerminalClosed(transport, req.SessionID, "failed to start shell: "+err.Error())
		return
	}

	sess := &TerminalSession{
		sessionID: req.SessionID,
		Ptmx:      ptmx,
		cmd:       cmd,
		done:      make(chan struct{}),
	}

	tm.Mu.Lock()
	tm.Sessions[req.SessionID] = sess
	tm.Mu.Unlock()

	// Notify hub that terminal is ready
	sendTerminalStartedWithTmux(transport, req.SessionID, tmuxAttached)

	// Stream PTY output → hub
	go tm.streamOutput(transport, sess)

	// Wait for process exit and clean up
	go func() {
		_ = cmd.Wait()
		close(sess.done)
		tm.cleanup(req.SessionID)
		SendTerminalClosed(transport, req.SessionID, "shell exited")
	}()
}

// HandleTerminalData writes incoming data to the PTY stdin.
func (tm *TerminalManager) HandleTerminalData(msg agentmgr.Message) {
	var payload agentmgr.TerminalDataPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		return
	}

	tm.Mu.Lock()
	sess, ok := tm.Sessions[payload.SessionID]
	tm.Mu.Unlock()
	if !ok {
		return
	}

	decoded, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil {
		return
	}

	_, _ = sess.Ptmx.Write(decoded)
}

// HandleTerminalResize changes the PTY window size.
func (tm *TerminalManager) HandleTerminalResize(msg agentmgr.Message) {
	var req agentmgr.TerminalResizeData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		return
	}

	tm.Mu.Lock()
	sess, ok := tm.Sessions[req.SessionID]
	tm.Mu.Unlock()
	if !ok {
		return
	}

	if req.Cols > 0 && req.Rows > 0 {
		_ = pty.Setsize(sess.Ptmx, &pty.Winsize{
			Cols: ClampUint16(req.Cols),
			Rows: ClampUint16(req.Rows),
		})
	}
}

func ClampUint16(value int) uint16 {
	if value <= 0 {
		return 0
	}
	if value > math.MaxUint16 {
		return math.MaxUint16
	}
	return uint16(value)
}

// HandleTerminalClose terminates a terminal session.
func (tm *TerminalManager) HandleTerminalClose(msg agentmgr.Message) {
	var req agentmgr.TerminalCloseData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		return
	}

	tm.Mu.Lock()
	sess, ok := tm.Sessions[req.SessionID]
	tm.Mu.Unlock()
	if !ok {
		return
	}

	// Close PTY — this will cause the shell process to exit
	_ = sess.Ptmx.Close()
	if sess.cmd.Process != nil {
		_ = sess.cmd.Process.Kill()
	}
}

func (tm *TerminalManager) HandleTerminalTmuxKill(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.TerminalTmuxKillData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("terminal: invalid tmux kill request: %v", err)
		return
	}

	sendResult := func(status, output string) {
		payload, marshalErr := json.Marshal(agentmgr.CommandResultData{
			JobID:     req.JobID,
			SessionID: req.SessionID,
			CommandID: req.CommandID,
			Status:    status,
			Output:    output,
		})
		if marshalErr != nil {
			log.Printf("terminal: failed to marshal tmux kill result for %s: %v", req.JobID, marshalErr)
			return
		}
		if sendErr := transport.Send(agentmgr.Message{
			Type: agentmgr.MsgCommandResult,
			ID:   req.JobID,
			Data: payload,
		}); sendErr != nil {
			log.Printf("terminal: failed to send tmux kill result for %s: %v", req.JobID, sendErr)
		}
	}

	tmuxSession := strings.TrimSpace(req.TmuxSession)
	if tmuxSession == "" {
		sendResult("failed", "tmux session name is required")
		return
	}

	tmuxPath, err := exec.LookPath("tmux")
	if err != nil || strings.TrimSpace(tmuxPath) == "" {
		sendResult("failed", "tmux not available")
		return
	}

	timeout := DefaultCommandTimeout
	if req.Timeout > 0 {
		timeout = time.Duration(req.Timeout) * time.Second
	}
	if timeout > MaxRemoteCommandTimeout {
		timeout = MaxRemoteCommandTimeout
	}

	checkCtx, checkCancel := context.WithTimeout(context.Background(), timeout)
	defer checkCancel()

	checkCmd, err := securityruntime.NewCommandContext(checkCtx, tmuxPath, "has-session", "-t", tmuxSession)
	if err != nil {
		sendResult("failed", err.Error())
		return
	}
	if output, err := checkCmd.CombinedOutput(); err != nil {
		trimmed := TruncateCommandOutput(output, MaxCommandOutputBytes)
		if checkCtx.Err() == context.DeadlineExceeded {
			sendResult("failed", "tmux session check timed out")
			return
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			sendResult("succeeded", "")
			return
		}
		if trimmed == "" {
			trimmed = err.Error()
		}
		sendResult("failed", trimmed)
		return
	}

	killCtx, killCancel := context.WithTimeout(context.Background(), timeout)
	defer killCancel()

	killCmd, err := securityruntime.NewCommandContext(killCtx, tmuxPath, "kill-session", "-t", tmuxSession)
	if err != nil {
		sendResult("failed", err.Error())
		return
	}
	output, err := killCmd.CombinedOutput()
	if err != nil {
		trimmed := TruncateCommandOutput(output, MaxCommandOutputBytes)
		if killCtx.Err() == context.DeadlineExceeded {
			sendResult("failed", "tmux session kill timed out")
			return
		}
		if trimmed == "" {
			trimmed = err.Error()
		}
		sendResult("failed", trimmed)
		return
	}
	sendResult("succeeded", TruncateCommandOutput(output, MaxCommandOutputBytes))
}

// CloseAll terminates all active sessions (called on agent shutdown).
func (tm *TerminalManager) CloseAll() {
	tm.Mu.Lock()
	defer tm.Mu.Unlock()
	for id, sess := range tm.Sessions {
		_ = sess.Ptmx.Close()
		if sess.cmd.Process != nil {
			_ = sess.cmd.Process.Kill()
		}
		delete(tm.Sessions, id)
	}
}

// streamOutput reads from the PTY and sends output to the hub.
func (tm *TerminalManager) streamOutput(transport MessageSender, sess *TerminalSession) {
	buf := make([]byte, 4096)
	for {
		n, err := sess.Ptmx.Read(buf)
		if n > 0 {
			encoded := base64.StdEncoding.EncodeToString(buf[:n])
			data, _ := json.Marshal(agentmgr.TerminalDataPayload{
				SessionID: sess.sessionID,
				Data:      encoded,
			})
			_ = transport.Send(agentmgr.Message{
				Type: agentmgr.MsgTerminalData,
				ID:   sess.sessionID,
				Data: data,
			})
		}
		if err != nil {
			if err != io.EOF {
				log.Printf("terminal: read error for session %s: %v", sess.sessionID, err)
			}
			return
		}
	}
}

func (tm *TerminalManager) cleanup(sessionID string) {
	tm.Mu.Lock()
	defer tm.Mu.Unlock()
	if sess, ok := tm.Sessions[sessionID]; ok {
		_ = sess.Ptmx.Close()
		delete(tm.Sessions, sessionID)
	}
}

func sendTerminalStartedWithTmux(transport MessageSender, sessionID string, tmuxAttached bool) {
	data, _ := json.Marshal(agentmgr.TerminalStartedData{
		SessionID:    sessionID,
		TmuxAttached: tmuxAttached,
	})
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgTerminalStarted,
		ID:   sessionID,
		Data: data,
	})
}

func SendTerminalClosed(transport MessageSender, sessionID, reason string) {
	data, _ := json.Marshal(agentmgr.TerminalCloseData{SessionID: sessionID, Reason: reason})
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgTerminalClosed,
		ID:   sessionID,
		Data: data,
	})
}

// DetectShell finds the best available shell on the system.
func DetectShell() string {
	// Prefer the user's configured shell (SHELL env var).
	if userShell := os.Getenv("SHELL"); userShell != "" {
		// #nosec G703 -- local SHELL env is trusted runtime input on the managed node.
		if _, err := os.Stat(userShell); err == nil {
			return userShell
		}
	}
	// On macOS, prefer zsh (default since Catalina).
	for _, shell := range []string{"/bin/zsh", "/bin/bash", "/bin/sh"} {
		if _, err := os.Stat(shell); err == nil {
			return shell
		}
	}
	// Fallback: try PATH lookup
	if path, err := exec.LookPath("zsh"); err == nil {
		return path
	}
	if path, err := exec.LookPath("bash"); err == nil {
		return path
	}
	if path, err := exec.LookPath("sh"); err == nil {
		return path
	}
	return "/bin/sh"
}
