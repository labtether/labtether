package operations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/terminal"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

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

func TestHandleExecMultiRateLimitPreventsDispatch(t *testing.T) {
	var executions atomic.Int32
	deps := newExecMultiTestDeps(t, []string{"srv1"}, func(terminal.CommandJob) terminal.CommandResult {
		executions.Add(1)
		return terminal.CommandResult{Status: "succeeded"}
	})
	deps.EnforceRateLimit = func(w http.ResponseWriter, _ *http.Request, bucket string, limit int, window time.Duration) bool {
		if bucket != execRateLimitBucket || limit != execRateLimitCount || window != execRateLimitWindow {
			t.Fatalf("rate policy = %q/%d/%s", bucket, limit, window)
		}
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return false
	}

	recorder := handleExecMultiTestRequest(t, deps, ExecMultiRequest{Targets: []string{"srv1"}, Command: "uptime"})
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429; body=%s", recorder.Code, recorder.Body.String())
	}
	if got := executions.Load(); got != 0 {
		t.Fatalf("executions = %d, want zero", got)
	}
}

func TestHandleExecMultiMaintenancePreflightPreventsAllDispatch(t *testing.T) {
	var executions atomic.Int32
	deps := newExecMultiTestDeps(t, []string{"srv1", "srv2", "srv3"}, func(terminal.CommandJob) terminal.CommandResult {
		executions.Add(1)
		return terminal.CommandResult{Status: "succeeded"}
	})
	deps.EvaluateAssetGuardrails = func(assetID string, _ time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
		return groupfeatures.GroupMaintenanceGuardrails{
			GroupID:      "group-1",
			BlockActions: assetID == "srv2",
		}, nil
	}

	recorder := handleExecMultiTestRequest(t, deps, ExecMultiRequest{
		Targets: []string{"srv1", "srv2", "srv3"},
		Command: "uptime",
	})
	if recorder.Code != http.StatusLocked || !strings.Contains(recorder.Body.String(), "maintenance_blocked") {
		t.Fatalf("status = %d, want 423 maintenance_blocked; body=%s", recorder.Code, recorder.Body.String())
	}
	if got := executions.Load(); got != 0 {
		t.Fatalf("executions = %d, want zero when any target is maintenance-blocked", got)
	}
}

func TestHandleExecMultiRejectsCommandDeniedByPolicy(t *testing.T) {
	var mu sync.Mutex
	executions := 0
	deps := newExecMultiTestDeps(t, []string{"srv1"}, func(terminal.CommandJob) terminal.CommandResult {
		mu.Lock()
		executions++
		mu.Unlock()
		return terminal.CommandResult{Status: "succeeded"}
	})
	deps.EvaluateCommandPolicy = func(context.Context, string, string) policy.CheckResponse {
		return policy.CheckResponse{Allowed: false, Reason: "command not in allowlist", Mode: "structured"}
	}

	recorder := handleExecMultiTestRequest(t, deps, ExecMultiRequest{
		Targets: []string{"srv1"},
		Command: "curl example.com",
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if executions != 0 {
		t.Fatalf("policy-denied multi-exec reached execution backend %d times", executions)
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
	if got := envelope.Data.Results["srv1"].Error; got != "policy_denied" {
		t.Fatalf("result error = %q, want policy_denied", got)
	}
	if envelope.Data.Summary["total"] != 1 || envelope.Data.Summary["failed"] != 1 {
		t.Fatalf("summary = %#v, want one failure", envelope.Data.Summary)
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
		EvaluateCommandPolicy: func(context.Context, string, string) policy.CheckResponse {
			return policy.CheckResponse{Allowed: true, Mode: "structured"}
		},
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
