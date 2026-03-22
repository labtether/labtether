package truenas

import (
	"reflect"
	"testing"

	"github.com/labtether/labtether/internal/connectorsdk"
)

func TestAnyToString(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{"string", "hello", "hello"},
		{"empty string", "", ""},
		{"float64", float64(42), "42"},
		{"float64 zero", float64(0), "0"},
		{"int", 7, "7"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"nil", nil, ""},
		{"fallback fmt", map[string]any{"k": "v"}, "map[k:v]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := anyToString(tt.input)
			if got != tt.want {
				t.Errorf("anyToString(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAnyToFloat(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	tests := []struct {
		name  string
		input any
		want  float64
	}{
		{"float64", float64(3.14), 3.14},
		{"float64 zero", float64(0), 0},
		{"int", 42, 42},
		{"int zero", 0, 0},
		{"string number", "123.45", 123.45},
		{"string zero", "0", 0},
		{"string empty", "", 0},
		{"string non-numeric", "abc", 0},
		{"nil", nil, 0},
		// TrueNAS nested value map — should unwrap
		{"nested rawvalue", map[string]any{"rawvalue": "1024"}, 1024},
		{"nested value", map[string]any{"value": float64(512)}, 512},
		{"nested zero rawvalue", map[string]any{"rawvalue": "0"}, 0},
		{"nested nil value", map[string]any{"value": nil, "rawvalue": "99"}, 99},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := anyToFloat(tt.input)
			if got != tt.want {
				t.Errorf("anyToFloat(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestAnyToBool(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	tests := []struct {
		name   string
		input  any
		want   bool
		wantOK bool
	}{
		{"bool true", true, true, true},
		{"bool false", false, false, true},
		{"string true", "true", true, true},
		{"string false", "false", false, true},
		{"string yes", "yes", true, true},
		{"string no", "no", false, true},
		{"string 1", "1", true, true},
		{"string 0", "0", false, true},
		{"string garbage", "maybe", false, false},
		{"float64 nonzero", float64(1), true, true},
		{"float64 zero", float64(0), false, true},
		{"int nonzero", 1, true, true},
		{"int zero", 0, false, true},
		{"nil", nil, false, false},
		// Nested TrueNAS map
		{"nested parsed true", map[string]any{"parsed": true}, true, true},
		{"nested value false", map[string]any{"value": "false"}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotOK := anyToBool(tt.input)
			if got != tt.want || gotOK != tt.wantOK {
				t.Errorf("anyToBool(%v) = (%v, %v), want (%v, %v)", tt.input, got, gotOK, tt.want, tt.wantOK)
			}
		})
	}
}

func TestNormalizeID(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "tank", "tank"},
		{"zfs path", "tank/data", "tank--data"},
		{"nested path", "tank/data/backups", "tank--data--backups"},
		{"spaces", "my pool", "my_pool"},
		{"dots preserved", "disk.0", "disk.0"},
		{"colon", "share:1", "share-1"},
		{"mixed", "TANK/Data Set.1", "tank--data_set.1"},
		{"empty", "", "unknown"},
		{"whitespace only", "   ", "unknown"},
		// Key test: these must NOT collide
		{"slash vs hyphen a", "tank/data", "tank--data"},
		{"slash vs hyphen b", "tank-data", "tank-data"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeID(tt.input)
			if got != tt.want {
				t.Errorf("normalizeID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}

	// Verify no collisions between common TrueNAS patterns
	a := normalizeID("tank/data")
	b := normalizeID("tank-data")
	if a == b {
		t.Errorf("normalizeID collision: %q and %q both produce %q", "tank/data", "tank-data", a)
	}

	c := normalizeID("tank/data.archive")
	d := normalizeID("tank/data-archive")
	if c == d {
		t.Errorf("normalizeID collision: %q and %q both produce %q", "tank/data.archive", "tank/data-archive", c)
	}
}

func TestNestedValue(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	tests := []struct {
		name string
		m    map[string]any
		keys []string
		want any
	}{
		{
			"simple key",
			map[string]any{"name": "tank"},
			[]string{"name"},
			"tank",
		},
		{
			"nested truenas value map",
			map[string]any{"mountpoint": map[string]any{"value": "/mnt/tank", "rawvalue": "/mnt/tank"}},
			[]string{"mountpoint"},
			"/mnt/tank",
		},
		{
			"missing key",
			map[string]any{"name": "tank"},
			[]string{"missing"},
			nil,
		},
		{
			"nil map",
			nil,
			[]string{"key"},
			nil,
		},
		{
			"non-map intermediate",
			map[string]any{"name": "tank"},
			[]string{"name", "child"},
			nil,
		},
		{
			"map without wrapper keys",
			map[string]any{"nested": map[string]any{"actual": "value"}},
			[]string{"nested"},
			map[string]any{"actual": "value"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nestedValue(tt.m, tt.keys...)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("nestedValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatFloat(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	tests := []struct {
		input float64
		want  string
	}{
		{0, "0"},
		{1024, "1024"},
		{1099511627776, "1099511627776"},
	}
	for _, tt := range tests {
		got := formatFloat(tt.input)
		if got != tt.want {
			t.Errorf("formatFloat(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParamOrTarget(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	if got := paramOrTarget(connectorsdk.ActionRequest{
		Params: map[string]string{"pool_name": "tank"},
	}, "pool_name"); got != "tank" {
		t.Fatalf("paramOrTarget() = %q, want tank", got)
	}
	if got := paramOrTarget(connectorsdk.ActionRequest{
		Params:   map[string]string{"pool_name": "  "},
		TargetID: "fallback-target",
	}, "pool_name"); got != "fallback-target" {
		t.Fatalf("paramOrTarget() fallback = %q, want fallback-target", got)
	}
}

func TestClampPercent(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	if got := clampPercent(-1); got != 0 {
		t.Fatalf("clampPercent(-1) = %v, want 0", got)
	}
	if got := clampPercent(120); got != 100 {
		t.Fatalf("clampPercent(120) = %v, want 100", got)
	}
	if got := clampPercent(42); got != 42 {
		t.Fatalf("clampPercent(42) = %v, want 42", got)
	}
}
