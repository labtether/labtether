package alerts

import (
	"testing"
)

func TestNormalizeSeverity(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"critical", SeverityCritical},
		{"CRITICAL", SeverityCritical},
		{"  high  ", SeverityHigh},
		{"Medium", SeverityMedium},
		{"LOW", SeverityLow},
		{"unknown", ""},
		{"", ""},
	}
	for _, tc := range cases {
		got := NormalizeSeverity(tc.input)
		if got != tc.want {
			t.Errorf("NormalizeSeverity(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestNormalizeRuleKind(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"metric_threshold", RuleKindMetricThreshold},
		{"METRIC_DEADMAN", RuleKindMetricDeadman},
		{"heartbeat_stale", RuleKindHeartbeatStale},
		{"log_pattern", RuleKindLogPattern},
		{"composite", RuleKindComposite},
		{"synthetic_check", RuleKindSyntheticCheck},
		{"bogus", ""},
	}
	for _, tc := range cases {
		got := NormalizeRuleKind(tc.input)
		if got != tc.want {
			t.Errorf("NormalizeRuleKind(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestNormalizeTargetScope(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"asset", TargetScopeAsset},
		{"GROUP", TargetScopeGroup},
		{"global", TargetScopeGlobal},
		{"", ""},
		{"universe", ""},
	}
	for _, tc := range cases {
		got := NormalizeTargetScope(tc.input)
		if got != tc.want {
			t.Errorf("NormalizeTargetScope(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestNormalizeEvaluationStatus(t *testing.T) {
	for _, v := range []string{EvaluationStatusOK, EvaluationStatusTriggered, EvaluationStatusSuppressed, EvaluationStatusError} {
		got := NormalizeEvaluationStatus(v)
		if got != v {
			t.Errorf("NormalizeEvaluationStatus(%q) = %q, want %q", v, got, v)
		}
	}
	if NormalizeEvaluationStatus("garbage") != "" {
		t.Error("expected empty string for unknown status")
	}
}

func TestNormalizeInstanceStatus(t *testing.T) {
	for _, v := range []string{InstanceStatusPending, InstanceStatusFiring, InstanceStatusAcknowledged, InstanceStatusResolved} {
		got := NormalizeInstanceStatus(v)
		if got != v {
			t.Errorf("NormalizeInstanceStatus(%q) = %q, want %q", v, got, v)
		}
	}
}

func TestCanTransitionInstanceStatus(t *testing.T) {
	// Valid transitions
	valid := [][2]string{
		{InstanceStatusPending, InstanceStatusFiring},
		{InstanceStatusPending, InstanceStatusResolved},
		{InstanceStatusFiring, InstanceStatusAcknowledged},
		{InstanceStatusFiring, InstanceStatusResolved},
		{InstanceStatusAcknowledged, InstanceStatusFiring},
		{InstanceStatusAcknowledged, InstanceStatusResolved},
		// Same-to-same is allowed
		{InstanceStatusPending, InstanceStatusPending},
		{InstanceStatusFiring, InstanceStatusFiring},
	}
	for _, pair := range valid {
		if !CanTransitionInstanceStatus(pair[0], pair[1]) {
			t.Errorf("expected %q -> %q to be valid", pair[0], pair[1])
		}
	}

	// Invalid transitions
	invalid := [][2]string{
		{InstanceStatusResolved, InstanceStatusFiring},
		{InstanceStatusResolved, InstanceStatusPending},
		{"", InstanceStatusFiring},
		{InstanceStatusFiring, "bogus"},
	}
	for _, pair := range invalid {
		if CanTransitionInstanceStatus(pair[0], pair[1]) {
			t.Errorf("expected %q -> %q to be invalid", pair[0], pair[1])
		}
	}
}

func TestTargetReferenceCount(t *testing.T) {
	cases := []struct {
		target RuleTargetInput
		want   int
	}{
		{RuleTargetInput{}, 0},
		{RuleTargetInput{AssetID: "a1"}, 1},
		{RuleTargetInput{GroupID: "g1"}, 1},
		{RuleTargetInput{Selector: map[string]any{"env": "prod"}}, 1},
		{RuleTargetInput{AssetID: "a1", GroupID: "g1"}, 2},
		{RuleTargetInput{AssetID: "a1", GroupID: "g1", Selector: map[string]any{"k": "v"}}, 3},
	}
	for _, tc := range cases {
		got := TargetReferenceCount(tc.target)
		if got != tc.want {
			t.Errorf("TargetReferenceCount(%+v) = %d, want %d", tc.target, got, tc.want)
		}
	}
}

func TestGenerateFingerprint(t *testing.T) {
	// Same inputs must produce the same fingerprint
	fp1 := GenerateFingerprint("rule-1", map[string]string{"host": "srv1", "env": "prod"})
	fp2 := GenerateFingerprint("rule-1", map[string]string{"env": "prod", "host": "srv1"}) // different insertion order
	if fp1 != fp2 {
		t.Errorf("fingerprints differ for same label set: %q vs %q", fp1, fp2)
	}

	// Different rule IDs must produce different fingerprints
	fp3 := GenerateFingerprint("rule-2", map[string]string{"host": "srv1", "env": "prod"})
	if fp1 == fp3 {
		t.Errorf("different rule IDs produced the same fingerprint: %q", fp1)
	}

	// No labels still produces a non-empty fingerprint
	fp4 := GenerateFingerprint("rule-1", nil)
	if fp4 == "" {
		t.Error("fingerprint with no labels should not be empty")
	}
	if fp4 == fp1 {
		t.Error("fingerprint with no labels should differ from fingerprint with labels")
	}
}
