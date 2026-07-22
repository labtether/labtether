package operations

import (
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/updates"
)

func TestExecuteUpdateScopeRejectsUnsupportedScope(t *testing.T) {
	deps := &UpdateExecutorDeps{}
	for _, dryRun := range []bool{false, true} {
		t.Run(map[bool]string{false: "apply", true: "dry-run"}[dryRun], func(t *testing.T) {
			entry := deps.ExecuteUpdateScope(updates.Job{DryRun: dryRun}, "asset-1", updates.ScopeDockerImage)
			if entry.Status != updates.StatusFailed {
				t.Fatalf("status = %q, want failed", entry.Status)
			}
			if !strings.Contains(entry.Summary, "not supported") || !strings.Contains(entry.Summary, "no changes applied") {
				t.Fatalf("summary = %q", entry.Summary)
			}
		})
	}
}

func TestExecuteUpdateWithExecutorRejectsUnsupportedScopeBeforeExecutor(t *testing.T) {
	called := false
	result := ExecuteUpdateWithExecutor(updates.Job{
		JobID: "job-1",
		RunID: "run-1",
		Plan: updates.Plan{
			Targets: []string{"asset-1"},
			Scopes:  []string{updates.ScopeDockerImage},
		},
	}, func(_ updates.Job, _, _ string) updates.RunResultEntry {
		called = true
		return updates.RunResultEntry{Status: updates.StatusSucceeded}
	})

	if called {
		t.Fatal("executor was called for an unsupported scope")
	}
	if result.Status != updates.StatusFailed || len(result.Results) != 1 || result.Results[0].Status != updates.StatusFailed {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteUpdateScopeDryRunUsesAgentPreviewWithoutApplying(t *testing.T) {
	mgr := agentmgr.NewManager()
	mgr.Register(&agentmgr.AgentConn{AssetID: "asset-1", Platform: "linux"})

	previewCalls := 0
	applyCalls := 0
	deps := &UpdateExecutorDeps{
		AgentMgr: mgr,
		PreviewOSPackageUpdatesViaAgent: func(requestID, target string, timeout time.Duration) agentmgr.CommandResultData {
			previewCalls++
			if !strings.HasPrefix(requestID, "updpreview_") {
				t.Fatalf("requestID = %q, want updpreview prefix", requestID)
			}
			if target != "asset-1" {
				t.Fatalf("target = %q, want asset-1", target)
			}
			if timeout != DefaultUpdatePreviewTimeout {
				t.Fatalf("timeout = %s, want %s", timeout, DefaultUpdatePreviewTimeout)
			}
			return agentmgr.CommandResultData{
				JobID:  requestID,
				Status: updates.StatusSucceeded,
				Output: "dry-run preview: no changes applied on asset-1; 1 OS package update(s) available: curl 8.5 -> 8.6",
			}
		},
		ExecuteUpdateViaAgent: func(string, string, string, []string, time.Duration, bool) agentmgr.CommandResultData {
			applyCalls++
			return agentmgr.CommandResultData{Status: updates.StatusSucceeded}
		},
	}

	entry := deps.ExecuteUpdateScope(updates.Job{DryRun: true}, "asset-1", updates.ScopeOSPackages)
	if entry.Status != updates.StatusSucceeded {
		t.Fatalf("status = %q, want succeeded; entry=%+v", entry.Status, entry)
	}
	if !strings.Contains(entry.Summary, "1 OS package update") || !strings.Contains(entry.Summary, "no changes applied") {
		t.Fatalf("summary = %q", entry.Summary)
	}
	if previewCalls != 1 {
		t.Fatalf("preview calls = %d, want 1", previewCalls)
	}
	if applyCalls != 0 {
		t.Fatalf("apply calls = %d, want 0", applyCalls)
	}
}

func TestExecuteUpdateScopeLiveRunStillUsesApplyRequest(t *testing.T) {
	mgr := agentmgr.NewManager()
	mgr.Register(&agentmgr.AgentConn{AssetID: "asset-1", Platform: "linux"})

	previewCalls := 0
	applyCalls := 0
	deps := &UpdateExecutorDeps{
		AgentMgr: mgr,
		PreviewOSPackageUpdatesViaAgent: func(string, string, time.Duration) agentmgr.CommandResultData {
			previewCalls++
			return agentmgr.CommandResultData{Status: updates.StatusSucceeded}
		},
		ExecuteUpdateViaAgent: func(requestID, target, mode string, packages []string, timeout time.Duration, force bool) agentmgr.CommandResultData {
			applyCalls++
			if !strings.HasPrefix(requestID, "updreq_") {
				t.Fatalf("requestID = %q, want updreq prefix", requestID)
			}
			if target != "asset-1" || mode != updates.ScopeOSPackages || len(packages) != 0 || timeout != DefaultUpdateAgentTimeout || force {
				t.Fatalf("unexpected apply request target=%q mode=%q packages=%v timeout=%s force=%v", target, mode, packages, timeout, force)
			}
			return agentmgr.CommandResultData{Status: updates.StatusSucceeded, Output: "packages upgraded"}
		},
	}

	entry := deps.ExecuteUpdateScope(updates.Job{DryRun: false}, "asset-1", updates.ScopeOSPackages)
	if entry.Status != updates.StatusSucceeded || entry.Summary != "packages upgraded" {
		t.Fatalf("entry = %+v", entry)
	}
	if previewCalls != 0 || applyCalls != 1 {
		t.Fatalf("preview calls=%d apply calls=%d, want 0/1", previewCalls, applyCalls)
	}
}

func TestExecuteUpdateScopeDryRunFailsClosedWithoutValidatedPreview(t *testing.T) {
	mgr := agentmgr.NewManager()
	mgr.Register(&agentmgr.AgentConn{AssetID: "asset-1", Platform: "linux"})

	tests := []struct {
		name    string
		preview func(string, string, time.Duration) agentmgr.CommandResultData
		want    string
	}{
		{name: "preview dependency unavailable", preview: nil, want: "preview unavailable"},
		{
			name: "agent rejects preview",
			preview: func(requestID, _ string, _ time.Duration) agentmgr.CommandResultData {
				return agentmgr.CommandResultData{JobID: requestID, Status: updates.StatusFailed, Output: "agent does not support upgradable package inventory; no changes applied"}
			},
			want: "does not support",
		},
		{
			name: "empty success is not evidence",
			preview: func(requestID, _ string, _ time.Duration) agentmgr.CommandResultData {
				return agentmgr.CommandResultData{JobID: requestID, Status: updates.StatusSucceeded}
			},
			want: "empty package update preview",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := &UpdateExecutorDeps{
				AgentMgr:                        mgr,
				PreviewOSPackageUpdatesViaAgent: test.preview,
			}
			entry := deps.ExecuteUpdateScope(updates.Job{DryRun: true}, "asset-1", updates.ScopeOSPackages)
			if entry.Status != updates.StatusFailed {
				t.Fatalf("status = %q, want failed; entry=%+v", entry.Status, entry)
			}
			if !strings.Contains(entry.Summary, test.want) || !strings.Contains(entry.Summary, "no changes applied") {
				t.Fatalf("summary = %q, want %q and safety statement", entry.Summary, test.want)
			}
		})
	}
}
