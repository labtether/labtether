package modelmap

import (
	"testing"

	"github.com/labtether/labtether/internal/connectorsdk"
)

func TestCanonicalOperationID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		actionID string
		want     string
	}{
		{actionID: "vm.start", want: "workload.start"},
		{actionID: "snapshot.delete", want: "snapshot.delete"},
		{actionID: "container.pause", want: "container.pause"},
		{actionID: "unknown.action", want: "unknown.action"},
	}

	for _, tc := range cases {
		if got := CanonicalOperationID(tc.actionID); got != tc.want {
			t.Fatalf("CanonicalOperationID(%q) = %q, want %q", tc.actionID, got, tc.want)
		}
	}
}

func TestCanonicalizeActionDescriptorsAddsCanonicalID(t *testing.T) {
	t.Parallel()

	actions := []connectorsdk.ActionDescriptor{
		{ID: "vm.start", Name: "Start VM"},
		{ID: "snapshot.create", Name: "Snapshot"},
	}

	got := CanonicalizeActionDescriptors(actions)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].CanonicalID != "workload.start" {
		t.Fatalf("got[0].CanonicalID = %q, want workload.start", got[0].CanonicalID)
	}
	if got[1].CanonicalID != "snapshot.create" {
		t.Fatalf("got[1].CanonicalID = %q, want snapshot.create", got[1].CanonicalID)
	}
}

func TestResolveActionID(t *testing.T) {
	t.Parallel()

	actions := []connectorsdk.ActionDescriptor{
		{ID: "vm.start", Name: "Start VM"},
		{ID: "ct.start", Name: "Start CT"},
		{ID: "container.start", Name: "Start Container"},
	}

	if got := ResolveActionID("vm.start", "proxmox-vm-100", actions); got != "vm.start" {
		t.Fatalf("exact action should resolve directly, got %q", got)
	}
	if got := ResolveActionID("workload.start", "proxmox-vm-100", actions); got != "vm.start" {
		t.Fatalf("canonical vm action should resolve to vm.start, got %q", got)
	}
	if got := ResolveActionID("workload.start", "proxmox-ct-101", actions); got != "ct.start" {
		t.Fatalf("canonical ct action should resolve to ct.start, got %q", got)
	}
	if got := ResolveActionID("workload.start", "docker-ct-node-a-abc123", actions); got != "container.start" {
		t.Fatalf("canonical container action should resolve to container.start, got %q", got)
	}
	if got := ResolveActionID("workload.start", "", actions); got == "" {
		t.Fatalf("expected deterministic fallback action id, got empty")
	}
}
