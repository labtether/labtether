package actions

import (
	"encoding/json"
	"testing"
	"time"
)

// --- NormalizeRunType ---

func TestNormalizeRunType_ValidTypes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"command", RunTypeCommand},
		{"connector_action", RunTypeConnectorAction},
		// case-insensitive
		{"COMMAND", RunTypeCommand},
		{"Connector_Action", RunTypeConnectorAction},
		// whitespace trimming
		{"  command  ", RunTypeCommand},
		{"  connector_action\t", RunTypeConnectorAction},
	}
	for _, tt := range tests {
		got := NormalizeRunType(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeRunType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeRunType_InvalidTypes(t *testing.T) {
	for _, input := range []string{"", "run", "action", "TERMINAL", "cmd", "  "} {
		got := NormalizeRunType(input)
		if got != "" {
			t.Errorf("NormalizeRunType(%q) = %q, want empty string", input, got)
		}
	}
}

// --- NormalizeStatus ---

func TestNormalizeStatus_ValidStatuses(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"queued", StatusQueued},
		{"running", StatusRunning},
		{"succeeded", StatusSucceeded},
		{"failed", StatusFailed},
		// case-insensitive
		{"QUEUED", StatusQueued},
		{"Running", StatusRunning},
		{"SUCCEEDED", StatusSucceeded},
		{"FAILED", StatusFailed},
		// whitespace trimming
		{"  queued  ", StatusQueued},
		{"\tsunning\n", ""}, // typo — should be empty
		{"  failed\t", StatusFailed},
	}
	for _, tt := range tests {
		got := NormalizeStatus(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeStatus(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeStatus_InvalidStatuses(t *testing.T) {
	for _, input := range []string{"", "pending", "done", "error", "cancelled", "  "} {
		got := NormalizeStatus(input)
		if got != "" {
			t.Errorf("NormalizeStatus(%q) = %q, want empty string", input, got)
		}
	}
}

// --- Constants sanity ---

func TestRunTypeConstants(t *testing.T) {
	// The constant values are part of the wire protocol — they must not change.
	if RunTypeCommand != "command" {
		t.Errorf("RunTypeCommand = %q, want \"command\"", RunTypeCommand)
	}
	if RunTypeConnectorAction != "connector_action" {
		t.Errorf("RunTypeConnectorAction = %q, want \"connector_action\"", RunTypeConnectorAction)
	}
}

func TestStatusConstants(t *testing.T) {
	if StatusQueued != "queued" {
		t.Errorf("StatusQueued = %q, want \"queued\"", StatusQueued)
	}
	if StatusRunning != "running" {
		t.Errorf("StatusRunning = %q, want \"running\"", StatusRunning)
	}
	if StatusSucceeded != "succeeded" {
		t.Errorf("StatusSucceeded = %q, want \"succeeded\"", StatusSucceeded)
	}
	if StatusFailed != "failed" {
		t.Errorf("StatusFailed = %q, want \"failed\"", StatusFailed)
	}
}

// --- ExecuteRequest JSON round-trip ---

func TestExecuteRequest_JSONRoundTrip(t *testing.T) {
	orig := ExecuteRequest{
		ActorID:     "user-123",
		Type:        RunTypeCommand,
		Target:      "asset-456",
		Command:     "df -h",
		ConnectorID: "",
		ActionID:    "",
		Params:      map[string]string{"env": "prod"},
		DryRun:      true,
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("json.Marshal(ExecuteRequest) error: %v", err)
	}

	var got ExecuteRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal(ExecuteRequest) error: %v", err)
	}

	if got.ActorID != orig.ActorID {
		t.Errorf("ActorID: got %q, want %q", got.ActorID, orig.ActorID)
	}
	if got.Type != orig.Type {
		t.Errorf("Type: got %q, want %q", got.Type, orig.Type)
	}
	if got.Target != orig.Target {
		t.Errorf("Target: got %q, want %q", got.Target, orig.Target)
	}
	if got.Command != orig.Command {
		t.Errorf("Command: got %q, want %q", got.Command, orig.Command)
	}
	if got.DryRun != orig.DryRun {
		t.Errorf("DryRun: got %v, want %v", got.DryRun, orig.DryRun)
	}
	if got.Params["env"] != "prod" {
		t.Errorf("Params[env]: got %q, want \"prod\"", got.Params["env"])
	}
}

func TestExecuteRequest_OmitEmptyFields(t *testing.T) {
	req := ExecuteRequest{
		ActorID: "user-1",
		Type:    RunTypeCommand,
		Command: "uptime",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	// omitempty fields should be absent when zero-valued
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal to map error: %v", err)
	}
	for _, field := range []string{"target", "connector_id", "action_id", "params", "dry_run"} {
		if _, present := raw[field]; present {
			t.Errorf("field %q should be omitted when empty, but was present in JSON", field)
		}
	}
}

// --- Run JSON round-trip ---

func TestRun_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	completed := now.Add(5 * time.Second)

	orig := Run{
		ID:          "run-abc",
		Type:        RunTypeConnectorAction,
		ActorID:     "user-99",
		Target:      "asset-1",
		ConnectorID: "conn-42",
		ActionID:    "act-7",
		Params:      map[string]string{"key": "value"},
		DryRun:      false,
		Status:      StatusSucceeded,
		Output:      "ok",
		Error:       "",
		CreatedAt:   now,
		UpdatedAt:   now,
		CompletedAt: &completed,
		Steps: []RunStep{
			{
				ID:        "step-1",
				RunID:     "run-abc",
				Name:      "pre-check",
				Status:    StatusSucceeded,
				Output:    "all good",
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("json.Marshal(Run) error: %v", err)
	}

	var got Run
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal(Run) error: %v", err)
	}

	if got.ID != orig.ID {
		t.Errorf("ID: got %q, want %q", got.ID, orig.ID)
	}
	if got.Status != orig.Status {
		t.Errorf("Status: got %q, want %q", got.Status, orig.Status)
	}
	if got.CompletedAt == nil {
		t.Fatal("CompletedAt: got nil, want non-nil")
	}
	if !got.CompletedAt.Equal(*orig.CompletedAt) {
		t.Errorf("CompletedAt: got %v, want %v", got.CompletedAt, orig.CompletedAt)
	}
	if len(got.Steps) != 1 {
		t.Fatalf("Steps: got %d, want 1", len(got.Steps))
	}
	if got.Steps[0].Name != "pre-check" {
		t.Errorf("Steps[0].Name: got %q, want \"pre-check\"", got.Steps[0].Name)
	}
}

func TestRun_NilCompletedAt_OmittedFromJSON(t *testing.T) {
	r := Run{
		ID:        "run-1",
		Type:      RunTypeCommand,
		ActorID:   "user-1",
		Status:    StatusQueued,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if _, present := raw["completed_at"]; present {
		t.Error("completed_at should be omitted when nil, but was present in JSON")
	}
}

// --- Job JSON round-trip ---

func TestJob_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)

	orig := Job{
		JobID:       "job-xyz",
		RunID:       "run-abc",
		Type:        RunTypeCommand,
		ActorID:     "user-1",
		Target:      "asset-2",
		Command:     "ls /tmp",
		Params:      map[string]string{"flag": "verbose"},
		DryRun:      true,
		RequestedAt: now,
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("json.Marshal(Job) error: %v", err)
	}

	var got Job
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal(Job) error: %v", err)
	}

	if got.JobID != orig.JobID {
		t.Errorf("JobID: got %q, want %q", got.JobID, orig.JobID)
	}
	if got.RunID != orig.RunID {
		t.Errorf("RunID: got %q, want %q", got.RunID, orig.RunID)
	}
	if got.DryRun != orig.DryRun {
		t.Errorf("DryRun: got %v, want %v", got.DryRun, orig.DryRun)
	}
	if !got.RequestedAt.Equal(orig.RequestedAt) {
		t.Errorf("RequestedAt: got %v, want %v", got.RequestedAt, orig.RequestedAt)
	}
}

// --- Result JSON round-trip ---

func TestResult_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)

	orig := Result{
		JobID:  "job-1",
		RunID:  "run-1",
		Status: StatusFailed,
		Error:  "exit code 1",
		Steps: []StepResult{
			{Name: "step-a", Status: StatusSucceeded, Output: "done"},
			{Name: "step-b", Status: StatusFailed, Error: "timeout"},
		},
		CompletedAt: now,
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("json.Marshal(Result) error: %v", err)
	}

	var got Result
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal(Result) error: %v", err)
	}

	if got.Status != orig.Status {
		t.Errorf("Status: got %q, want %q", got.Status, orig.Status)
	}
	if got.Error != orig.Error {
		t.Errorf("Error: got %q, want %q", got.Error, orig.Error)
	}
	if len(got.Steps) != 2 {
		t.Fatalf("Steps: got %d, want 2", len(got.Steps))
	}
	if got.Steps[1].Error != "timeout" {
		t.Errorf("Steps[1].Error: got %q, want \"timeout\"", got.Steps[1].Error)
	}
	if !got.CompletedAt.Equal(orig.CompletedAt) {
		t.Errorf("CompletedAt: got %v, want %v", got.CompletedAt, orig.CompletedAt)
	}
}

// --- StepResult omitempty ---

func TestStepResult_OmitEmptyOutputAndError(t *testing.T) {
	sr := StepResult{
		Name:   "no-output-step",
		Status: StatusSucceeded,
	}

	data, err := json.Marshal(sr)
	if err != nil {
		t.Fatalf("json.Marshal(StepResult) error: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	for _, field := range []string{"output", "error"} {
		if _, present := raw[field]; present {
			t.Errorf("field %q should be omitted when empty, but was present in JSON", field)
		}
	}
}

// --- NormalizeRunType and NormalizeStatus are inverses of the constants ---

func TestNormalizeRunType_RoundTripsConstants(t *testing.T) {
	for _, rt := range []string{RunTypeCommand, RunTypeConnectorAction} {
		if got := NormalizeRunType(rt); got != rt {
			t.Errorf("NormalizeRunType(%q) = %q, want identity", rt, got)
		}
	}
}

func TestNormalizeStatus_RoundTripsConstants(t *testing.T) {
	for _, s := range []string{StatusQueued, StatusRunning, StatusSucceeded, StatusFailed} {
		if got := NormalizeStatus(s); got != s {
			t.Errorf("NormalizeStatus(%q) = %q, want identity", s, got)
		}
	}
}
