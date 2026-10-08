package main

import (
	"encoding/json"
	"errors"
	"github.com/labtether/labtether/internal/agentmgr"
	"golang.org/x/crypto/ssh"
	"testing"
	"time"
)

func TestProbeAgentTmuxUsesCachedConnectionMetadata(t *testing.T) {
	var srv apiServer
	conn := &agentmgr.AgentConn{AssetID: "node-1"}
	conn.SetMeta("terminal.tmux.has", "true")
	conn.SetMeta("terminal.tmux.path", "/usr/bin/tmux")

	resp := srv.probeAgentTmux(conn)
	if !resp.HasTmux {
		t.Fatal("expected cached tmux capability to be used")
	}
	if resp.TmuxPath != "/usr/bin/tmux" {
		t.Fatalf("unexpected tmux path: %q", resp.TmuxPath)
	}
}

func TestProcessAgentTerminalProbedCachesProbeResultOnConnection(t *testing.T) {
	var srv apiServer
	conn := &agentmgr.AgentConn{AssetID: "node-1"}

	payload, err := json.Marshal(agentmgr.TerminalProbeResponse{
		HasTmux:  true,
		TmuxPath: "/bin/tmux",
	})
	if err != nil {
		t.Fatalf("marshal terminal probe payload: %v", err)
	}

	srv.processAgentTerminalProbed(conn, agentmgr.Message{Data: payload})

	if got := conn.Meta("terminal.tmux.has"); got != "true" {
		t.Fatalf("expected cached tmux flag true, got %q", got)
	}
	if got := conn.Meta("terminal.tmux.path"); got != "/bin/tmux" {
		t.Fatalf("expected cached tmux path /bin/tmux, got %q", got)
	}
	if got := conn.Meta("terminal.tmux.probe_pending"); got != "false" {
		t.Fatalf("expected probe pending flag reset to false, got %q", got)
	}
}

func TestStartAgentTmuxProbeAsyncResetsPendingOnSendFailure(t *testing.T) {
	var srv apiServer
	conn := &agentmgr.AgentConn{AssetID: "node-1"}

	if !srv.startAgentTmuxProbeAsync(conn) {
		t.Fatal("expected probe dispatch to be accepted")
	}
	if got := conn.Meta("terminal.tmux.probe_pending"); got != "true" {
		t.Fatalf("expected probe pending flag true immediately, got %q", got)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if conn.Meta("terminal.tmux.probe_pending") == "false" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("expected probe pending flag to reset after async send failure")
}

func TestDialSSHWithRetryRetriesOnceThenSucceeds(t *testing.T) {
	baseConfig := &ssh.ClientConfig{
		User:            "tester",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	seenTimeouts := make([]time.Duration, 0, 2)
	seenAttempts := make([]int, 0, 2)
	sleepCalls := 0
	dialCalls := 0

	client, attemptsUsed, err := dialSSHWithRetry(
		"127.0.0.1:22",
		baseConfig,
		3*time.Second,
		2,
		150*time.Millisecond,
		func(_ string, _ string, cfg *ssh.ClientConfig) (*ssh.Client, error) {
			dialCalls++
			seenTimeouts = append(seenTimeouts, cfg.Timeout)
			if dialCalls == 1 {
				return nil, errors.New("dial timeout")
			}
			return &ssh.Client{}, nil
		},
		func(delay time.Duration) {
			sleepCalls++
			if delay != 150*time.Millisecond {
				t.Fatalf("unexpected retry delay: %s", delay)
			}
		},
		func(attempt, _ int) {
			seenAttempts = append(seenAttempts, attempt)
		},
	)
	if err != nil {
		t.Fatalf("expected retry dial success, got error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client on successful retry")
	}
	if attemptsUsed != 2 {
		t.Fatalf("expected success on attempt 2, got %d", attemptsUsed)
	}
	if dialCalls != 2 {
		t.Fatalf("expected 2 dial calls, got %d", dialCalls)
	}
	if sleepCalls != 1 {
		t.Fatalf("expected one retry sleep, got %d", sleepCalls)
	}
	if len(seenAttempts) != 2 || seenAttempts[0] != 1 || seenAttempts[1] != 2 {
		t.Fatalf("unexpected attempt callback sequence: %v", seenAttempts)
	}
	if len(seenTimeouts) != 2 || seenTimeouts[0] != 3*time.Second || seenTimeouts[1] != 3*time.Second {
		t.Fatalf("unexpected timeout values: %v", seenTimeouts)
	}
	if baseConfig.Timeout != 0 {
		t.Fatalf("expected base config timeout to remain unchanged, got %s", baseConfig.Timeout)
	}
}

func TestDialSSHWithRetryReturnsLastErrorAfterMaxAttempts(t *testing.T) {
	baseConfig := &ssh.ClientConfig{
		User:            "tester",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	sleepCalls := 0
	dialCalls := 0

	client, attemptsUsed, err := dialSSHWithRetry(
		"127.0.0.1:22",
		baseConfig,
		2*time.Second,
		2,
		100*time.Millisecond,
		func(_ string, _ string, _ *ssh.ClientConfig) (*ssh.Client, error) {
			dialCalls++
			return nil, errors.New("connection refused")
		},
		func(_ time.Duration) {
			sleepCalls++
		},
		nil,
	)
	if err == nil {
		t.Fatal("expected retry dial to fail")
	}
	if client != nil {
		t.Fatal("expected nil client on failure")
	}
	if attemptsUsed != 2 {
		t.Fatalf("expected max attempts used to be 2, got %d", attemptsUsed)
	}
	if dialCalls != 2 {
		t.Fatalf("expected 2 dial calls, got %d", dialCalls)
	}
	if sleepCalls != 1 {
		t.Fatalf("expected one retry sleep, got %d", sleepCalls)
	}
}
