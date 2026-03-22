package terminal

import (
	"strings"
	"testing"
)

func TestRingBuffer_BasicWrite(t *testing.T) {
	rb := NewRingBuffer(5) // 5 lines max
	rb.Write([]byte("line1\nline2\nline3\n"))
	snap := rb.Snapshot()
	if rb.Lines() != 3 {
		t.Fatalf("expected 3 lines, got %d", rb.Lines())
	}
	if string(snap) != "line1\nline2\nline3\n" {
		t.Fatalf("unexpected snapshot: %q", string(snap))
	}
}

func TestRingBuffer_Eviction(t *testing.T) {
	rb := NewRingBuffer(3) // 3 lines max
	rb.Write([]byte("a\nb\nc\nd\ne\n"))
	if rb.Lines() != 3 {
		t.Fatalf("expected 3 lines, got %d", rb.Lines())
	}
	snap := string(rb.Snapshot())
	if !strings.HasPrefix(snap, "c\n") {
		t.Fatalf("expected oldest line 'c', got: %q", snap)
	}
}

func TestRingBuffer_PartialLines(t *testing.T) {
	rb := NewRingBuffer(10)
	rb.Write([]byte("partial"))
	rb.Write([]byte(" line\ncomplete\n"))
	if rb.Lines() != 2 {
		t.Fatalf("expected 2 lines, got %d", rb.Lines())
	}
	snap := string(rb.Snapshot())
	if snap != "partial line\ncomplete\n" {
		t.Fatalf("unexpected snapshot: %q", snap)
	}
}

func TestRingBuffer_EmptySnapshot(t *testing.T) {
	rb := NewRingBuffer(10)
	snap := rb.Snapshot()
	if len(snap) != 0 {
		t.Fatalf("expected empty snapshot, got %d bytes", len(snap))
	}
	if rb.Lines() != 0 {
		t.Fatalf("expected 0 lines, got %d", rb.Lines())
	}
}

func TestRingBuffer_LargeEviction(t *testing.T) {
	rb := NewRingBuffer(3)
	for i := 0; i < 100; i++ {
		rb.Write([]byte("line\n"))
	}
	if rb.Lines() != 3 {
		t.Fatalf("expected 3 lines after 100 writes, got %d", rb.Lines())
	}
}
