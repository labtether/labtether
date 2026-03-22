package idgen

import (
	"strings"
	"testing"
)

func TestNew_Format(t *testing.T) {
	id := New("test")
	parts := strings.SplitN(id, "_", 3)
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts (prefix_nanotime_counter), got %d: %q", len(parts), id)
	}
	if parts[0] != "test" {
		t.Errorf("prefix = %q, want %q", parts[0], "test")
	}
	// Nanotime and counter must be numeric.
	for _, part := range parts[1:] {
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				t.Errorf("non-numeric character %q in ID: %q", string(ch), id)
			}
		}
	}
}

func TestNew_Uniqueness(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		id := New("u")
		if seen[id] {
			t.Fatalf("duplicate ID generated: %q", id)
		}
		seen[id] = true
	}
}

func TestNew_Monotonic(t *testing.T) {
	prev := New("m")
	for i := 0; i < 100; i++ {
		curr := New("m")
		if curr <= prev {
			t.Fatalf("IDs not monotonically increasing: %q >= %q", prev, curr)
		}
		prev = curr
	}
}

func TestNew_PrefixPreserved(t *testing.T) {
	prefixes := []string{"jq", "sess", "cmd", "evt", "job"}
	for _, prefix := range prefixes {
		id := New(prefix)
		if !strings.HasPrefix(id, prefix+"_") {
			t.Errorf("ID %q does not start with %q", id, prefix+"_")
		}
	}
}
