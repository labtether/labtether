package agentcore

import "testing"

func TestRingBufferPushAndDrain(t *testing.T) {
	rb := NewRingBuffer[int](5)

	rb.Push(1)
	rb.Push(2)
	rb.Push(3)

	if rb.Len() != 3 {
		t.Fatalf("expected len 3, got %d", rb.Len())
	}

	items := rb.Drain()
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if items[0] != 1 || items[1] != 2 || items[2] != 3 {
		t.Fatalf("expected [1,2,3], got %v", items)
	}

	if rb.Len() != 0 {
		t.Fatalf("expected empty after drain, got len %d", rb.Len())
	}
}

func TestRingBufferOverwrite(t *testing.T) {
	rb := NewRingBuffer[int](3)

	rb.Push(1)
	rb.Push(2)
	rb.Push(3)
	rb.Push(4)
	rb.Push(5)

	if rb.Len() != 3 {
		t.Fatalf("expected len 3, got %d", rb.Len())
	}

	items := rb.Drain()
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	// Oldest items (1, 2) should be overwritten.
	if items[0] != 3 || items[1] != 4 || items[2] != 5 {
		t.Fatalf("expected [3,4,5], got %v", items)
	}
}

func TestRingBufferDrainEmpty(t *testing.T) {
	rb := NewRingBuffer[string](10)
	items := rb.Drain()
	if items != nil {
		t.Fatalf("expected nil from empty drain, got %v", items)
	}
}

func TestRingBufferDefaultCapacity(t *testing.T) {
	rb := NewRingBuffer[int](0)
	// Default should be 60.
	for i := 0; i < 70; i++ {
		rb.Push(i)
	}
	if rb.Len() != 60 {
		t.Fatalf("expected len 60, got %d", rb.Len())
	}

	items := rb.Drain()
	// Should have items 10-69 (oldest 0-9 overwritten).
	if items[0] != 10 {
		t.Fatalf("expected first item 10, got %d", items[0])
	}
	if items[59] != 69 {
		t.Fatalf("expected last item 69, got %d", items[59])
	}
}

func TestRingBufferWrapAround(t *testing.T) {
	rb := NewRingBuffer[int](4)

	// Fill, drain, fill again to test wrap-around.
	rb.Push(1)
	rb.Push(2)
	rb.Push(3)
	rb.Push(4)
	_ = rb.Drain()

	rb.Push(5)
	rb.Push(6)

	items := rb.Drain()
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0] != 5 || items[1] != 6 {
		t.Fatalf("expected [5,6], got %v", items)
	}
}
