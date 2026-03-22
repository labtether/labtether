package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/runtimesettings"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/labtether/labtether/internal/updates"
)

func TestExecuteCommandSuccess(t *testing.T) {
	result := executeCommand(terminal.CommandJob{
		JobID:       "job_1",
		SessionID:   "sess_1",
		CommandID:   "cmd_1",
		Target:      "lab-host-01",
		Command:     "uptime",
		RequestedAt: time.Now().UTC(),
	})

	if result.Status != "succeeded" {
		t.Fatalf("expected succeeded status, got %s", result.Status)
	}
}

func TestExecuteCommandFailure(t *testing.T) {
	result := executeCommand(terminal.CommandJob{
		JobID:       "job_2",
		SessionID:   "sess_1",
		CommandID:   "cmd_2",
		Target:      "lab-host-01",
		Command:     "force-fail-now",
		RequestedAt: time.Now().UTC(),
	})

	if result.Status != "failed" {
		t.Fatalf("expected failed status, got %s", result.Status)
	}
}

func TestExecuteActionCommand(t *testing.T) {
	result := executeActionInProcess(actions.Job{
		JobID:   "job_action_1",
		RunID:   "actrun_1",
		Type:    actions.RunTypeCommand,
		Target:  "lab-host-01",
		Command: "uname -a",
	}, nil)

	if result.Status != actions.StatusSucceeded {
		t.Fatalf("expected succeeded status, got %s", result.Status)
	}
}

func TestExecuteUpdateDryRun(t *testing.T) {
	result := executeUpdate(updates.Job{
		JobID:  "job_update_1",
		RunID:  "uprun_1",
		DryRun: true,
		Plan: updates.Plan{
			ID:      "upln_1",
			Name:    "Weekly",
			Targets: []string{"lab-host-01"},
			Scopes:  []string{updates.ScopeOSPackages},
		},
	})

	if result.Status != updates.StatusSucceeded {
		t.Fatalf("expected succeeded status, got %s", result.Status)
	}
	if len(result.Results) != 1 {
		t.Fatalf("expected one result row, got %d", len(result.Results))
	}
}

func TestExecuteUpdateWithExecutorAggregatesFailures(t *testing.T) {
	job := updates.Job{
		JobID:  "job_update_2",
		RunID:  "uprun_2",
		DryRun: false,
		Plan: updates.Plan{
			ID:      "upln_2",
			Name:    "Weekly",
			Targets: []string{"lab-host-01"},
			Scopes:  []string{updates.ScopeOSPackages, updates.ScopeDockerImage},
		},
	}

	result := executeUpdateWithExecutor(job, func(_ updates.Job, target, scope string) updates.RunResultEntry {
		if scope == updates.ScopeOSPackages {
			return updates.RunResultEntry{
				Target:  target,
				Scope:   scope,
				Status:  updates.StatusSucceeded,
				Summary: "updated packages",
			}
		}
		return updates.RunResultEntry{
			Target:  target,
			Scope:   scope,
			Status:  updates.StatusFailed,
			Summary: "scope not implemented",
		}
	})

	if result.Status != updates.StatusFailed {
		t.Fatalf("expected failed status, got %s", result.Status)
	}
	if len(result.Results) != 2 {
		t.Fatalf("expected two result rows, got %d", len(result.Results))
	}
}

func TestParseSSHTargetUserHostPort(t *testing.T) {
	target, err := parseSSHTarget("alice@lab-host-01:2222", "", 22)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if target.User != "alice" {
		t.Fatalf("expected user alice, got %s", target.User)
	}
	if target.Host != "lab-host-01" {
		t.Fatalf("expected host lab-host-01, got %s", target.Host)
	}
	if target.Port != 2222 {
		t.Fatalf("expected port 2222, got %d", target.Port)
	}
}

func TestParseSSHTargetURL(t *testing.T) {
	target, err := parseSSHTarget("ssh://bob@10.0.0.5:2200", "root", 22)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if target.User != "bob" {
		t.Fatalf("expected user bob, got %s", target.User)
	}
	if target.Host != "10.0.0.5" {
		t.Fatalf("expected host 10.0.0.5, got %s", target.Host)
	}
	if target.Port != 2200 {
		t.Fatalf("expected port 2200, got %d", target.Port)
	}
}

func TestParseSSHTargetRequiresUser(t *testing.T) {
	_, err := parseSSHTarget("10.0.0.5:22", "", 22)
	if err == nil {
		t.Fatalf("expected error when ssh user is missing")
	}
}

func TestResolveJobSSHConfigUsesJobConfig(t *testing.T) {
	job := terminal.CommandJob{
		Target: "ignored-target",
		SSHConfig: &terminal.SSHConfig{
			Host: "192.168.1.20",
			Port: 2222,
			User: "labuser",
		},
	}

	resolved, err := resolveJobSSHConfig(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Host != "192.168.1.20" {
		t.Fatalf("expected host 192.168.1.20, got %s", resolved.Host)
	}
	if resolved.Port != 2222 {
		t.Fatalf("expected port 2222, got %d", resolved.Port)
	}
	if resolved.User != "labuser" {
		t.Fatalf("expected user labuser, got %s", resolved.User)
	}
}

func TestResolveJobSSHConfigFallsBackToTarget(t *testing.T) {
	t.Setenv("SSH_USERNAME", "owner")
	job := terminal.CommandJob{
		Target: "10.0.0.5:2200",
	}

	resolved, err := resolveJobSSHConfig(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Host != "10.0.0.5" {
		t.Fatalf("expected host 10.0.0.5, got %s", resolved.Host)
	}
	if resolved.Port != 2200 {
		t.Fatalf("expected port 2200, got %d", resolved.Port)
	}
	if resolved.User != "owner" {
		t.Fatalf("expected user owner, got %s", resolved.User)
	}
}

func TestTruncateOutput(t *testing.T) {
	payload := []byte(strings.Repeat("x", 256))
	out := truncateOutput(payload, 64)
	if !strings.Contains(out, "output truncated") {
		t.Fatalf("expected truncated output marker, got: %s", out)
	}
}

func TestRefreshWorkerRuntimeSettingsDirectWiresQueueMaxDeliveries(t *testing.T) {
	store := persistence.NewMemoryRuntimeSettingsStore()
	if _, err := store.SaveRuntimeSettingOverrides(map[string]string{
		runtimesettings.KeyWorkerQueueMaxDeliveries: "9",
	}); err != nil {
		t.Fatalf("save runtime overrides: %v", err)
	}

	state := newWorkerRuntimeState(5, time.Minute)
	queue := jobqueue.New(nil, time.Second, 5)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	applied := make(chan struct{}, 1)
	go refreshWorkerRuntimeSettingsDirect(ctx, store, state, func(s *workerRuntimeState) {
		queue.SetMaxAttempts(uint64ToIntClamp(s.MaxDeliveries()))
		select {
		case applied <- struct{}{}:
		default:
		}
		cancel()
	})

	select {
	case <-applied:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for runtime settings apply")
	}

	if got := state.MaxDeliveries(); got != 9 {
		t.Fatalf("expected state max deliveries 9, got %d", got)
	}
	if got := queue.MaxAttempts(); got != 9 {
		t.Fatalf("expected queue max attempts 9, got %d", got)
	}
}

func TestConfiguredJobWorkerCountClampsMinimum(t *testing.T) {
	t.Setenv("JOB_WORKERS", "0")
	if got := configuredJobWorkerCount(); got != 1 {
		t.Fatalf("expected worker count to clamp to 1, got %d", got)
	}
}

func TestConfiguredJobWorkerCountUsesConfiguredValue(t *testing.T) {
	t.Setenv("JOB_WORKERS", "7")
	if got := configuredJobWorkerCount(); got != 7 {
		t.Fatalf("expected configured worker count 7, got %d", got)
	}
}

func TestLoadPolicyConfigFromEnvUsesProductDefaults(t *testing.T) {
	t.Setenv("STRUCTURED_ENABLED", "")
	t.Setenv("INTERACTIVE_ENABLED", "")
	t.Setenv("CONNECTOR_ENABLED", "")
	t.Setenv("COMMAND_ALLOWLIST_MODE", "")

	cfg := loadPolicyConfigFromEnv()
	if !cfg.StructuredEnabled {
		t.Fatal("expected structured mode enabled by default")
	}
	if !cfg.InteractiveEnabled {
		t.Fatal("expected interactive mode enabled by default")
	}
	if !cfg.ConnectorEnabled {
		t.Fatal("expected connector mode enabled by default")
	}
	if !cfg.AllowlistMode {
		t.Fatal("expected allowlist mode enabled by default")
	}
}
