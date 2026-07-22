package operations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/terminal"
)

func newConnectedExecDeps(t *testing.T, captured *terminal.CommandJob) *ExecDeps {
	t.Helper()

	agentMgr := agentmgr.NewManager()
	agentMgr.Register(agentmgr.NewAgentConn(nil, "srv1", "linux"))

	return &ExecDeps{
		AgentMgr:   agentMgr,
		AssetStore: persistence.NewMemoryAssetStore(),
		ExecuteViaAgent: func(job terminal.CommandJob) terminal.CommandResult {
			*captured = job
			return terminal.CommandResult{Status: "succeeded", Output: "ok"}
		},
		PrincipalActorID:         func(context.Context) string { return "tester" },
		AllowedAssetsFromContext: func(context.Context) []string { return nil },
	}
}

func TestExecuteLocalCommandUsesLiteralArgv(t *testing.T) {
	t.Setenv("LABTETHER_EXEC_ALLOWLIST_MODE", "true")
	t.Setenv("LABTETHER_EXEC_ALLOWED_BINARIES", "printf")
	t.Setenv("LABTETHER_SHELL_COMMAND_ALLOWLIST_MODE", "true")
	t.Setenv("LABTETHER_SHELL_COMMAND_ALLOWLIST_PREFIXES", "printf")

	output, err := ExecuteLocalCommand(terminal.CommandJob{
		Command: `printf "%s" "hello world"`,
	}, CommandExecutorConfig{Mode: ExecutorModeLocal, Timeout: time.Second, MaxOutputBytes: 1024})
	if err != nil {
		t.Fatalf("execute literal argv: %v", err)
	}
	if output != "hello world" {
		t.Fatalf("output = %q, want literal argument output", output)
	}

	if _, err := ExecuteLocalCommand(terminal.CommandJob{
		Command: `printf ok; id`,
	}, CommandExecutorConfig{Mode: ExecutorModeLocal, Timeout: time.Second, MaxOutputBytes: 1024}); err == nil {
		t.Fatal("expected shell operator injection to be rejected")
	}
}

func TestCommandExecutorDefaultsToDisabledAndFailsClosed(t *testing.T) {
	t.Setenv("TERMINAL_EXECUTOR_MODE", "")
	cfg := LoadCommandExecutorConfig()
	if cfg.Mode != ExecutorModeDisabled {
		t.Fatalf("mode = %q, want %q", cfg.Mode, ExecutorModeDisabled)
	}
	status, output := ExecuteConfiguredCommand(terminal.CommandJob{Target: "offline-asset", Command: "uptime"}, cfg)
	if status != "failed" || !strings.Contains(output, "unavailable") {
		t.Fatalf("status=%q output=%q", status, output)
	}
}

func TestCommandExecutorSimulationRequiresExplicitMode(t *testing.T) {
	t.Setenv("TERMINAL_EXECUTOR_MODE", ExecutorModeSimulated)
	cfg := LoadCommandExecutorConfig()
	if cfg.Mode != ExecutorModeSimulated {
		t.Fatalf("mode = %q, want %q", cfg.Mode, ExecutorModeSimulated)
	}
	status, output := ExecuteConfiguredCommand(terminal.CommandJob{Target: "fixture", Command: "uptime"}, cfg)
	if status != "succeeded" || !strings.Contains(output, "simulated") {
		t.Fatalf("status=%q output=%q", status, output)
	}
}

func TestCappedOutputWriterDrainsConcurrentWritesWithoutGrowing(t *testing.T) {
	const limit = 1024
	writer := newCappedOutputWriter(limit)
	payload := bytes.Repeat([]byte("x"), 16*1024)
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 8 {
				if written, err := writer.Write(payload); err != nil || written != len(payload) {
					t.Errorf("Write = %d, %v", written, err)
				}
			}
		}()
	}
	wg.Wait()
	writer.mu.Lock()
	retained := len(writer.data)
	total := writer.total
	writer.mu.Unlock()
	if retained != limit {
		t.Fatalf("retained=%d, want %d", retained, limit)
	}
	if total != int64(16*8*len(payload)) {
		t.Fatalf("total=%d, want %d", total, 16*8*len(payload))
	}
	if output := writer.String(); !strings.Contains(output, "output truncated") {
		t.Fatalf("missing truncation marker: %q", output)
	}
}

func TestExecuteLocalCommandCapsInfiniteOutputDuringExecution(t *testing.T) {
	t.Setenv("LABTETHER_EXEC_ALLOWLIST_MODE", "true")
	t.Setenv("LABTETHER_EXEC_ALLOWED_BINARIES", "yes")
	t.Setenv("LABTETHER_SHELL_COMMAND_ALLOWLIST_MODE", "true")
	t.Setenv("LABTETHER_SHELL_COMMAND_ALLOWLIST_PREFIXES", "yes")
	output, err := ExecuteLocalCommand(terminal.CommandJob{Command: "yes x"}, CommandExecutorConfig{
		Mode: ExecutorModeLocal, Timeout: 50 * time.Millisecond, MaxOutputBytes: 1024,
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout, got %v", err)
	}
	if !strings.Contains(output, "output truncated") {
		t.Fatalf("expected bounded truncation marker, got %q", output)
	}
	if len(output) > 1200 {
		t.Fatalf("bounded output length=%d", len(output))
	}
}

func TestExecOnAssetPassesTimeoutToAgentCommand(t *testing.T) {
	var captured terminal.CommandJob
	deps := newConnectedExecDeps(t, &captured)
	req := httptest.NewRequest("POST", "/api/v2/assets/srv1/exec", nil)

	result := deps.ExecOnAsset(req, "srv1", "uptime", 42)
	if result.Error != "" {
		t.Fatalf("ExecOnAsset returned error: %s", result.Error)
	}
	if captured.TimeoutSec != 42 {
		t.Fatalf("TimeoutSec = %d, want 42", captured.TimeoutSec)
	}
}

func TestExecOnAssetNormalizesTimeoutBounds(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  int
		want int
	}{
		{name: "default", raw: 0, want: DefaultExecTimeout},
		{name: "negative", raw: -10, want: DefaultExecTimeout},
		{name: "max", raw: MaxExecTimeout + 1, want: MaxExecTimeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var captured terminal.CommandJob
			deps := newConnectedExecDeps(t, &captured)
			req := httptest.NewRequest("POST", "/api/v2/assets/srv1/exec", nil)

			result := deps.ExecOnAsset(req, "srv1", "uptime", tc.raw)
			if result.Error != "" {
				t.Fatalf("ExecOnAsset returned error: %s", result.Error)
			}
			if captured.TimeoutSec != tc.want {
				t.Fatalf("TimeoutSec = %d, want %d", captured.TimeoutSec, tc.want)
			}
		})
	}
}

func TestHandleAssetExecRejectsAssetOutsideAPIKeyAllowlist(t *testing.T) {
	dispatched := false
	var captured terminal.CommandJob
	deps := newConnectedExecDeps(t, &captured)
	deps.ExecuteViaAgent = func(terminal.CommandJob) terminal.CommandResult {
		dispatched = true
		return terminal.CommandResult{Status: "succeeded"}
	}
	deps.DecodeJSONBody = func(w http.ResponseWriter, r *http.Request, dst any) error {
		return json.NewDecoder(r.Body).Decode(dst)
	}
	deps.ScopesFromContext = func(context.Context) []string { return []string{"assets:exec"} }
	deps.AllowedAssetsFromContext = func(context.Context) []string { return []string{"different-asset"} }

	req := httptest.NewRequest(http.MethodPost, "/api/v2/assets/srv1/exec", strings.NewReader(`{"command":"uptime"}`))
	rec := httptest.NewRecorder()
	deps.HandleAssetExec(rec, req, "srv1")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if dispatched {
		t.Fatal("restricted asset command reached execution backend")
	}
}

func TestHandleAssetExecAuditExcludesRawCommand(t *testing.T) {
	var captured terminal.CommandJob
	deps := newConnectedExecDeps(t, &captured)
	deps.DecodeJSONBody = func(w http.ResponseWriter, r *http.Request, dst any) error {
		return json.NewDecoder(r.Body).Decode(dst)
	}
	deps.ScopesFromContext = func(context.Context) []string { return []string{"assets:exec"} }
	var event AuditEvent
	deps.AppendAuditEventBestEffort = func(got AuditEvent, _ string) { event = got }
	secretCommand := "printf LTQA_V2_EXEC_SECRET_3d11"

	req := httptest.NewRequest(http.MethodPost, "/api/v2/assets/srv1/exec", bytes.NewBufferString(`{"command":"`+secretCommand+`"}`))
	rec := httptest.NewRecorder()
	deps.HandleAssetExec(rec, req, "srv1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secretCommand) || strings.Contains(string(encoded), "LTQA_V2_EXEC_SECRET_3d11") {
		t.Fatalf("v2 exec audit persisted raw command: %s", encoded)
	}
	if got := event.Details["command_bytes"]; got != len([]byte(secretCommand)) {
		t.Fatalf("command_bytes = %v, want %d", got, len([]byte(secretCommand)))
	}
}

func TestHandleExecMultiNormalizesAndDeduplicatesTargets(t *testing.T) {
	var mu sync.Mutex
	calls := make(map[string]int)
	deps := newExecMultiTestDeps(t, []string{"srv1"}, func(job terminal.CommandJob) terminal.CommandResult {
		mu.Lock()
		calls[job.Target]++
		mu.Unlock()
		return terminal.CommandResult{Status: "succeeded", Output: "ok"}
	})

	recorder := handleExecMultiTestRequest(t, deps, ExecMultiRequest{
		Targets: []string{" srv1 ", "srv1", "", "  "},
		Command: "uptime",
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if calls["srv1"] != 1 || len(calls) != 1 {
		t.Fatalf("execution calls = %#v, want srv1 exactly once", calls)
	}

	var envelope struct {
		Data struct {
			Results map[string]ExecResult `json:"results"`
			Summary map[string]int        `json:"summary"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(envelope.Data.Results) != 1 || envelope.Data.Results["srv1"].AssetID != "srv1" {
		t.Fatalf("results = %#v, want one normalized srv1 result", envelope.Data.Results)
	}
	if envelope.Data.Summary["total"] != 1 || envelope.Data.Summary["succeeded"] != 1 {
		t.Fatalf("summary = %#v, want one success", envelope.Data.Summary)
	}
}

func TestHandleExecMultiCountsNonzeroExitAsFailure(t *testing.T) {
	deps := newExecMultiTestDeps(t, []string{"good", "bad"}, func(job terminal.CommandJob) terminal.CommandResult {
		if job.Target == "bad" {
			return terminal.CommandResult{Status: "failed", Output: "command exited unsuccessfully"}
		}
		return terminal.CommandResult{Status: "succeeded", Output: "ok"}
	})

	recorder := handleExecMultiTestRequest(t, deps, ExecMultiRequest{
		Targets: []string{"good", "bad"},
		Command: "fixture",
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}

	var envelope struct {
		Data struct {
			Results map[string]ExecResult `json:"results"`
			Summary map[string]int        `json:"summary"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Data.Results["bad"].ExitCode == 0 {
		t.Fatalf("failed command result = %#v, want nonzero exit", envelope.Data.Results["bad"])
	}
	if envelope.Data.Summary["total"] != 2 || envelope.Data.Summary["succeeded"] != 1 || envelope.Data.Summary["failed"] != 1 {
		t.Fatalf("summary = %#v, want one success and one failure", envelope.Data.Summary)
	}
}

func TestHandleExecMultiRejectsExcessiveRawTargetsBeforeExecution(t *testing.T) {
	var mu sync.Mutex
	executions := 0
	deps := newExecMultiTestDeps(t, []string{"srv1"}, func(terminal.CommandJob) terminal.CommandResult {
		mu.Lock()
		executions++
		mu.Unlock()
		return terminal.CommandResult{Status: "succeeded"}
	})
	targets := make([]string, maxExecMultiRawTargets+1)
	for index := range targets {
		targets[index] = "srv1"
	}

	recorder := handleExecMultiTestRequest(t, deps, ExecMultiRequest{Targets: targets, Command: "uptime"})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
	if executions != 0 {
		t.Fatalf("execution backend called %d times for excessive raw targets", executions)
	}
}

func TestHandleExecMultiCapsConcurrency(t *testing.T) {
	targets := make([]string, 24)
	for index := range targets {
		targets[index] = fmt.Sprintf("node-%02d", index)
	}

	var mu sync.Mutex
	active := 0
	maxActive := 0
	executions := 0
	deps := newExecMultiTestDeps(t, targets, func(terminal.CommandJob) terminal.CommandResult {
		mu.Lock()
		active++
		executions++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()

		time.Sleep(20 * time.Millisecond)

		mu.Lock()
		active--
		mu.Unlock()
		return terminal.CommandResult{Status: "succeeded"}
	})

	recorder := handleExecMultiTestRequest(t, deps, ExecMultiRequest{Targets: targets, Command: "uptime"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if executions != len(targets) {
		t.Fatalf("execution count = %d, want %d", executions, len(targets))
	}
	if maxActive > maxExecMultiConcurrency {
		t.Fatalf("max concurrency = %d, limit = %d", maxActive, maxExecMultiConcurrency)
	}
}

func TestHandleExecMultiCapsExpandedGroupBeforeExecution(t *testing.T) {
	targets := make([]string, maxExecMultiUniqueTargets+1)
	store := persistence.NewMemoryAssetStore()
	for index := range targets {
		target := fmt.Sprintf("group-node-%02d", index)
		targets[index] = target
		if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: target,
			Name:    target,
			Type:    "host",
			Source:  "agent",
			Status:  "online",
			GroupID: "large-group",
		}); err != nil {
			t.Fatalf("create group asset %s: %v", target, err)
		}
	}

	var mu sync.Mutex
	executions := 0
	deps := newExecMultiTestDeps(t, targets, func(terminal.CommandJob) terminal.CommandResult {
		mu.Lock()
		executions++
		mu.Unlock()
		return terminal.CommandResult{Status: "succeeded"}
	})
	deps.AssetStore = store

	recorder := handleExecMultiTestRequest(t, deps, ExecMultiRequest{Group: " large-group ", Command: "uptime"})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
	if executions != 0 {
		t.Fatalf("execution backend called %d times for oversized group", executions)
	}
}

func newExecMultiTestDeps(
	t *testing.T,
	connectedTargets []string,
	execute func(terminal.CommandJob) terminal.CommandResult,
) *ExecDeps {
	t.Helper()

	agentMgr := agentmgr.NewManager()
	for _, target := range connectedTargets {
		agentMgr.Register(agentmgr.NewAgentConn(nil, target, "linux"))
	}
	return &ExecDeps{
		AgentMgr:        agentMgr,
		AssetStore:      persistence.NewMemoryAssetStore(),
		ExecuteViaAgent: execute,
		DecodeJSONBody: func(_ http.ResponseWriter, r *http.Request, dst any) error {
			return json.NewDecoder(r.Body).Decode(dst)
		},
		PrincipalActorID:         func(context.Context) string { return "tester" },
		AllowedAssetsFromContext: func(context.Context) []string { return nil },
		ScopesFromContext:        func(context.Context) []string { return nil },
	}
}

func handleExecMultiTestRequest(t *testing.T, deps *ExecDeps, request ExecMultiRequest) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal multi-exec request: %v", err)
	}
	httpRequest := httptest.NewRequest(http.MethodPost, "/api/v2/exec", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	deps.HandleExecMulti(recorder, httpRequest)
	return recorder
}
