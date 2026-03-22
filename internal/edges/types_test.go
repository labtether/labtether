package edges

import "testing"

func TestNormalizeOrigin(t *testing.T) {
	tests := []struct{ input, want string }{
		{"auto", OriginAuto},
		{"manual", OriginManual},
		{"suggested", OriginSuggested},
		{"dismissed", OriginDismissed},
		{"MANUAL", OriginManual},
		{"unknown", OriginManual}, // default
		{"", OriginManual},
	}
	for _, tt := range tests {
		if got := NormalizeOrigin(tt.input); got != tt.want {
			t.Errorf("NormalizeOrigin(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAggregateConfidence(t *testing.T) {
	tests := []struct {
		name   string
		scores []float64
		want   float64
	}{
		{"single", []float64{0.85}, 0.85},
		{"two signals", []float64{0.85, 0.70}, 0.955},
		{"empty", []float64{}, 0.0},
	}
	for _, tt := range tests {
		got := AggregateConfidence(tt.scores)
		if got < tt.want-0.001 || got > tt.want+0.001 {
			t.Errorf("AggregateConfidence(%v) = %f, want %f", tt.scores, got, tt.want)
		}
	}
}
