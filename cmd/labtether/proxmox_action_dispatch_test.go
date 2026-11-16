package main

import (
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/actions"
)

// This test stays in cmd while the other Proxmox action tests moved to
// internal/hubapi/proxmox: a command run reaches agent dispatch through cmd's
// worker wiring, which the package test builder does not provide.

func TestExecuteActionInProcessFallbackBranch(t *testing.T) {
	sut := newTestAPIServer(t)
	result := sut.executeActionInProcess(actions.Job{
		JobID:   "job-cmd-1",
		RunID:   "run-cmd-1",
		Type:    actions.RunTypeCommand,
		Target:  "host-1",
		Command: "echo ok",
	})
	if result.Status != actions.StatusFailed || !strings.Contains(result.Error, "not connected") {
		t.Fatalf("expected disconnected command-run fallback to fail closed, got %+v", result)
	}
}
