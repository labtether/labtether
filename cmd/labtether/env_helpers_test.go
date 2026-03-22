package main

import (
	"math"
	"testing"
	"time"
)

// Tests exercise the local aliases defined in env_helpers.go, which delegate to
// internal/hubapi/shared. The goal is end-to-end coverage of the aliased
// behaviour from the cmd/labtether package boundary.

func TestEnvHelperEnvOrDefault(t *testing.T) {
	t.Run("returns value when env set", func(t *testing.T) {
		t.Setenv("LT_TEST_ENVORDEFAULT", "hello")
		if got := envOrDefault("LT_TEST_ENVORDEFAULT", "fallback"); got != "hello" {
			t.Fatalf("expected hello, got %q", got)
		}
	})

	t.Run("returns fallback when env unset", func(t *testing.T) {
		t.Setenv("LT_TEST_ENVORDEFAULT_UNSET", "")
		if got := envOrDefault("LT_TEST_ENVORDEFAULT_UNSET", "fallback"); got != "fallback" {
			t.Fatalf("expected fallback, got %q", got)
		}
	})

	t.Run("returns fallback when env missing", func(t *testing.T) {
		// Key deliberately not set in this subtest.
		if got := envOrDefault("LT_TEST_ENVORDEFAULT_MISSING_KEY_XYZ", "default"); got != "default" {
			t.Fatalf("expected default, got %q", got)
		}
	})
}

func TestEnvHelperEnvOrDefaultInt(t *testing.T) {
	cases := []struct {
		name     string
		envValue string
		fallback int
		want     int
	}{
		{"valid positive integer", "42", 10, 42},
		{"empty value uses fallback", "", 10, 10},
		{"zero uses fallback (zero treated as invalid)", "0", 10, 10},
		{"negative uses fallback", "-5", 10, 10},
		{"non-numeric uses fallback", "abc", 10, 10},
		{"whitespace-padded valid integer", "  7  ", 10, 7},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LT_TEST_INT", tc.envValue)
			if got := envOrDefaultInt("LT_TEST_INT", tc.fallback); got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
}

func TestEnvHelperEnvOrDefaultUint64(t *testing.T) {
	cases := []struct {
		name     string
		envValue string
		fallback uint64
		want     uint64
	}{
		{"valid value", "100", 5, 100},
		{"empty uses fallback", "", 5, 5},
		{"zero uses fallback", "0", 5, 5},
		{"non-numeric uses fallback", "xyz", 5, 5},
		{"whitespace-padded valid value", " 99 ", 5, 99},
		{"large valid uint64", "18446744073709551615", 5, 18446744073709551615},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LT_TEST_UINT64", tc.envValue)
			if got := envOrDefaultUint64("LT_TEST_UINT64", tc.fallback); got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
}

func TestEnvHelperEnvOrDefaultDuration(t *testing.T) {
	cases := []struct {
		name     string
		envValue string
		fallback time.Duration
		want     time.Duration
	}{
		{"valid duration", "5m", time.Minute, 5 * time.Minute},
		{"empty uses fallback", "", time.Second, time.Second},
		{"invalid string uses fallback", "notaduration", time.Second, time.Second},
		{"zero duration uses fallback", "0s", time.Second, time.Second},
		{"negative duration uses fallback", "-1m", time.Second, time.Second},
		{"whitespace-padded valid duration", "  2h  ", time.Minute, 2 * time.Hour},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LT_TEST_DUR", tc.envValue)
			if got := envOrDefaultDuration("LT_TEST_DUR", tc.fallback); got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestEnvHelperEnvOrDefaultBool(t *testing.T) {
	cases := []struct {
		name     string
		envValue string
		fallback bool
		want     bool
	}{
		{"true literal", "true", false, true},
		{"false literal", "false", true, false},
		{"1 parses as true", "1", false, true},
		{"0 parses as false", "0", true, false},
		{"empty uses fallback true", "", true, true},
		{"empty uses fallback false", "", false, false},
		{"invalid string uses fallback", "maybe", true, true},
		{"whitespace-padded true", "  true  ", false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LT_TEST_BOOL", tc.envValue)
			if got := envOrDefaultBool("LT_TEST_BOOL", tc.fallback); got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestEnvHelperUint64ToIntClamp(t *testing.T) {
	cases := []struct {
		name  string
		input uint64
		want  int
	}{
		{"zero", 0, 0},
		{"small value", 42, 42},
		{"max int fits", uint64(math.MaxInt), math.MaxInt},
		{"overflow clamps to MaxInt", math.MaxUint64, math.MaxInt},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := uint64ToIntClamp(tc.input); got != tc.want {
				t.Fatalf("uint64ToIntClamp(%d): expected %d, got %d", tc.input, tc.want, got)
			}
		})
	}
}

func TestEnvHelperIntToUint64NonNegative(t *testing.T) {
	cases := []struct {
		name  string
		input int
		want  uint64
	}{
		{"positive value", 10, 10},
		{"zero returns 0", 0, 0},
		{"negative returns 0", -5, 0},
		{"large positive", math.MaxInt, uint64(math.MaxInt)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := intToUint64NonNegative(tc.input); got != tc.want {
				t.Fatalf("intToUint64NonNegative(%d): expected %d, got %d", tc.input, tc.want, got)
			}
		})
	}
}
