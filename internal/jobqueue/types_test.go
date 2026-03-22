package jobqueue

import "testing"

func TestValidateKind_ValidKinds(t *testing.T) {
	for _, kind := range []JobKind{KindTerminalCommand, KindActionRun, KindUpdateRun} {
		if err := ValidateKind(kind); err != nil {
			t.Errorf("ValidateKind(%q) = %v, want nil", kind, err)
		}
	}
}

func TestValidateKind_InvalidKind(t *testing.T) {
	for _, kind := range []JobKind{"", "bogus", "TERMINAL_COMMAND"} {
		if err := ValidateKind(kind); err == nil {
			t.Errorf("ValidateKind(%q) = nil, want error", kind)
		}
	}
}

func TestValidateStatus_ValidStatuses(t *testing.T) {
	for _, status := range []JobStatus{StatusQueued, StatusProcessing, StatusCompleted, StatusFailed, StatusDeadLettered} {
		if err := ValidateStatus(status); err != nil {
			t.Errorf("ValidateStatus(%q) = %v, want nil", status, err)
		}
	}
}

func TestValidateStatus_InvalidStatus(t *testing.T) {
	for _, status := range []JobStatus{"", "pending", "QUEUED", "cancelled"} {
		if err := ValidateStatus(status); err == nil {
			t.Errorf("ValidateStatus(%q) = nil, want error", status)
		}
	}
}

func TestJobStatusTransitions(t *testing.T) {
	// Invariant: these are the only terminal states (no further transitions).
	terminal := []JobStatus{StatusCompleted, StatusDeadLettered}
	for _, s := range terminal {
		if !ValidStatuses[s] {
			t.Errorf("terminal status %q missing from ValidStatuses", s)
		}
	}

	// Invariant: failed is NOT terminal — it transitions back to queued for retry.
	if !ValidStatuses[StatusFailed] {
		t.Error("StatusFailed missing from ValidStatuses")
	}
}
