package operations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/terminal"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
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
		EvaluateCommandPolicy: func(context.Context, string, string) policy.CheckResponse {
			return policy.CheckResponse{Allowed: true, Mode: "structured"}
		},
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

func TestExecuteLocalCommandCapsOutputDuringExecution(t *testing.T) {
	t.Setenv("LABTETHER_EXEC_ALLOWLIST_MODE", "true")
	t.Setenv("LABTETHER_EXEC_ALLOWED_BINARIES", "printf")
	t.Setenv("LABTETHER_SHELL_COMMAND_ALLOWLIST_MODE", "true")
	t.Setenv("LABTETHER_SHELL_COMMAND_ALLOWLIST_PREFIXES", "printf")
	payload := strings.Repeat("x", 16*1024)
	output, err := ExecuteLocalCommand(terminal.CommandJob{Command: "printf %s " + payload}, CommandExecutorConfig{
		Mode: ExecutorModeLocal, Timeout: 5 * time.Second, MaxOutputBytes: 1024,
	})
	if err != nil {
		t.Fatalf("execute bounded output command: %v", err)
	}
	if !strings.Contains(output, "output truncated") {
		t.Fatalf("expected bounded truncation marker, got %q", output)
	}
	if len(output) > 1200 {
		t.Fatalf("bounded output length=%d", len(output))
	}
}

func TestExecuteLocalCommandTimesOutInfiniteOutputWithinBound(t *testing.T) {
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

func TestHandleAssetExecRateLimitPreventsDispatch(t *testing.T) {
	var captured terminal.CommandJob
	deps := newConnectedExecDeps(t, &captured)
	dispatched := false
	deps.ExecuteViaAgent = func(terminal.CommandJob) terminal.CommandResult {
		dispatched = true
		return terminal.CommandResult{Status: "succeeded"}
	}
	deps.DecodeJSONBody = func(_ http.ResponseWriter, r *http.Request, dst any) error {
		return json.NewDecoder(r.Body).Decode(dst)
	}
	deps.ScopesFromContext = func(context.Context) []string { return []string{"assets:exec"} }
	deps.EnforceRateLimit = func(w http.ResponseWriter, _ *http.Request, bucket string, limit int, window time.Duration) bool {
		if bucket != execRateLimitBucket || limit != execRateLimitCount || window != execRateLimitWindow {
			t.Fatalf("rate policy = %q/%d/%s", bucket, limit, window)
		}
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return false
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/assets/srv1/exec", strings.NewReader(`{"command":"uptime"}`))
	rec := httptest.NewRecorder()
	deps.HandleAssetExec(rec, req, "srv1")

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429; body=%s", rec.Code, rec.Body.String())
	}
	if dispatched {
		t.Fatal("rate-limited command reached execution backend")
	}
}

func TestHandleAssetExecMaintenancePreventsPolicyAndDispatch(t *testing.T) {
	var captured terminal.CommandJob
	deps := newConnectedExecDeps(t, &captured)
	dispatched := false
	policyChecks := 0
	deps.ExecuteViaAgent = func(terminal.CommandJob) terminal.CommandResult {
		dispatched = true
		return terminal.CommandResult{Status: "succeeded"}
	}
	deps.DecodeJSONBody = func(_ http.ResponseWriter, r *http.Request, dst any) error {
		return json.NewDecoder(r.Body).Decode(dst)
	}
	deps.ScopesFromContext = func(context.Context) []string { return []string{"assets:exec"} }
	deps.EvaluateCommandPolicy = func(context.Context, string, string) policy.CheckResponse {
		policyChecks++
		return policy.CheckResponse{Allowed: true, Mode: "structured"}
	}
	deps.EvaluateAssetGuardrails = func(assetID string, _ time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
		if assetID != "srv1" {
			t.Fatalf("guard target = %q, want srv1", assetID)
		}
		return groupfeatures.GroupMaintenanceGuardrails{GroupID: "group-1", BlockActions: true}, nil
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/assets/srv1/exec", strings.NewReader(`{"command":"uptime"}`))
	rec := httptest.NewRecorder()
	deps.HandleAssetExec(rec, req, "srv1")

	if rec.Code != http.StatusLocked || !strings.Contains(rec.Body.String(), "maintenance_blocked") {
		t.Fatalf("status = %d, want 423 maintenance_blocked; body=%s", rec.Code, rec.Body.String())
	}
	if policyChecks != 0 || dispatched {
		t.Fatalf("policy checks = %d, dispatched = %v; want neither", policyChecks, dispatched)
	}
}

func TestHandleAssetExecFailsClosedWhenMaintenanceEvaluationFails(t *testing.T) {
	var captured terminal.CommandJob
	deps := newConnectedExecDeps(t, &captured)
	dispatched := false
	deps.ExecuteViaAgent = func(terminal.CommandJob) terminal.CommandResult {
		dispatched = true
		return terminal.CommandResult{Status: "succeeded"}
	}
	deps.DecodeJSONBody = func(_ http.ResponseWriter, r *http.Request, dst any) error {
		return json.NewDecoder(r.Body).Decode(dst)
	}
	deps.ScopesFromContext = func(context.Context) []string { return []string{"assets:exec"} }
	deps.EvaluateAssetGuardrails = func(string, time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
		return groupfeatures.GroupMaintenanceGuardrails{}, errors.New("store unavailable")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/assets/srv1/exec", strings.NewReader(`{"command":"uptime"}`))
	rec := httptest.NewRecorder()
	deps.HandleAssetExec(rec, req, "srv1")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rec.Code, rec.Body.String())
	}
	if dispatched {
		t.Fatal("command reached execution backend after maintenance evaluation failed")
	}
}

func TestHandleAssetExecRejectsCommandDeniedByPolicy(t *testing.T) {
	dispatched := false
	var captured terminal.CommandJob
	deps := newConnectedExecDeps(t, &captured)
	deps.ExecuteViaAgent = func(terminal.CommandJob) terminal.CommandResult {
		dispatched = true
		return terminal.CommandResult{Status: "succeeded"}
	}
	deps.DecodeJSONBody = func(_ http.ResponseWriter, r *http.Request, dst any) error {
		return json.NewDecoder(r.Body).Decode(dst)
	}
	deps.ScopesFromContext = func(context.Context) []string { return []string{"assets:exec"} }
	deps.EvaluateCommandPolicy = func(context.Context, string, string) policy.CheckResponse {
		return policy.CheckResponse{Allowed: false, Reason: "command not in allowlist", Mode: "structured"}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/assets/srv1/exec", strings.NewReader(`{"command":"curl example.com"}`))
	rec := httptest.NewRecorder()
	deps.HandleAssetExec(rec, req, "srv1")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if dispatched {
		t.Fatal("policy-denied command reached execution backend")
	}
}

func TestHandleAssetExecFailsClosedWithoutCommandPolicy(t *testing.T) {
	dispatched := false
	var captured terminal.CommandJob
	deps := newConnectedExecDeps(t, &captured)
	deps.ExecuteViaAgent = func(terminal.CommandJob) terminal.CommandResult {
		dispatched = true
		return terminal.CommandResult{Status: "succeeded"}
	}
	deps.DecodeJSONBody = func(_ http.ResponseWriter, r *http.Request, dst any) error {
		return json.NewDecoder(r.Body).Decode(dst)
	}
	deps.ScopesFromContext = func(context.Context) []string { return []string{"assets:exec"} }
	deps.EvaluateCommandPolicy = nil

	req := httptest.NewRequest(http.MethodPost, "/api/v2/assets/srv1/exec", strings.NewReader(`{"command":"uptime"}`))
	rec := httptest.NewRecorder()
	deps.HandleAssetExec(rec, req, "srv1")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
	if dispatched {
		t.Fatal("command reached execution backend without a policy evaluator")
	}
}

func TestHandleAssetExecDoesNotSkipPolicyForOfflineAsset(t *testing.T) {
	dispatched := false
	var captured terminal.CommandJob
	deps := newConnectedExecDeps(t, &captured)
	deps.AgentMgr = agentmgr.NewManager()
	deps.ExecuteViaAgent = func(terminal.CommandJob) terminal.CommandResult {
		dispatched = true
		return terminal.CommandResult{Status: "succeeded"}
	}
	deps.DecodeJSONBody = func(_ http.ResponseWriter, r *http.Request, dst any) error {
		return json.NewDecoder(r.Body).Decode(dst)
	}
	deps.ScopesFromContext = func(context.Context) []string { return []string{"assets:exec"} }
	policyChecks := 0
	deps.EvaluateCommandPolicy = func(context.Context, string, string) policy.CheckResponse {
		policyChecks++
		return policy.CheckResponse{Allowed: false, Reason: "command not in allowlist", Mode: "structured"}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/assets/srv1/exec", strings.NewReader(`{"command":"curl example.com"}`))
	rec := httptest.NewRecorder()
	deps.HandleAssetExec(rec, req, "srv1")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if policyChecks != 1 {
		t.Fatalf("policy checks = %d, want 1", policyChecks)
	}
	if dispatched {
		t.Fatal("offline policy-denied command reached execution backend")
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
