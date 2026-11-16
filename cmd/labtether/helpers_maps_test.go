package main

import (
	"testing"

	"github.com/labtether/labtether/internal/hubapi/shared"
)

// Tests cover the local aliases defined in helpers_maps.go.

// ---------------------------------------------------------------------------
// maxRuntimeCacheEntries constant
// ---------------------------------------------------------------------------

func TestHelperMapMaxRuntimeCacheEntries(t *testing.T) {
	// Sanity-check that the constant is positive and within a sensible range.
	if shared.MaxRuntimeCacheEntries <= 0 {
		t.Fatalf("maxRuntimeCacheEntries must be positive, got %d", shared.MaxRuntimeCacheEntries)
	}
	// The implementation documents a "generous" cap. Ensure it is at least 50
	// to provide meaningful headroom for multi-collector deployments.
	const minExpected = 50
	if shared.MaxRuntimeCacheEntries < minExpected {
		t.Fatalf("maxRuntimeCacheEntries=%d is unexpectedly small (want >= %d)", shared.MaxRuntimeCacheEntries, minExpected)
	}
}

// ---------------------------------------------------------------------------
// cloneAnyMap
// ---------------------------------------------------------------------------

func TestHelperMapCloneAnyMap(t *testing.T) {
	t.Run("nil input returns empty map", func(t *testing.T) {
		got := shared.CloneAnyMap(nil)
		if got == nil {
			t.Fatal("expected non-nil map for nil input")
		}
		if len(got) != 0 {
			t.Fatalf("expected empty map for nil input, got %v", got)
		}
	})

	t.Run("empty input returns empty map", func(t *testing.T) {
		got := shared.CloneAnyMap(map[string]any{})
		if len(got) != 0 {
			t.Fatalf("expected empty map, got %v", got)
		}
	})

	t.Run("key-value pairs copied", func(t *testing.T) {
		input := map[string]any{
			"alpha": 1,
			"beta":  "value",
			"gamma": true,
		}
		got := shared.CloneAnyMap(input)
		if len(got) != len(input) {
			t.Fatalf("expected %d entries, got %d", len(input), len(got))
		}
		for k, v := range input {
			if got[k] != v {
				t.Errorf("key %q: expected %v, got %v", k, v, got[k])
			}
		}
	})

	t.Run("result is a distinct map (mutations do not affect original)", func(t *testing.T) {
		input := map[string]any{"key": "original"}
		got := shared.CloneAnyMap(input)
		got["key"] = "mutated"
		if input["key"] != "original" {
			t.Fatal("cloneAnyMap returned the same map reference; original was mutated")
		}
	})

	t.Run("keys are trimmed of surrounding whitespace", func(t *testing.T) {
		input := map[string]any{
			"  spaced  ": "value",
		}
		got := shared.CloneAnyMap(input)
		if _, ok := got["spaced"]; !ok {
			t.Fatalf("expected key 'spaced' after trimming, got keys: %v", got)
		}
		if _, ok := got["  spaced  "]; ok {
			t.Fatal("expected untrimmed key to be absent from cloned map")
		}
	})

	t.Run("nil values preserved", func(t *testing.T) {
		input := map[string]any{"nullkey": nil}
		got := shared.CloneAnyMap(input)
		v, exists := got["nullkey"]
		if !exists {
			t.Fatal("expected 'nullkey' to be present in cloned map")
		}
		if v != nil {
			t.Fatalf("expected nil value, got %v", v)
		}
	})
}
