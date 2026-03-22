package agentcore

import "sync"

// RingBuffer is a bounded circular buffer that overwrites the oldest entries
// when full. Used to buffer telemetry samples during WebSocket disconnects.
type RingBuffer[T any] struct {
	mu   sync.Mutex
	data []T
	head int
	size int
	cap  int
}

// NewRingBuffer creates a ring buffer with the given capacity.
func NewRingBuffer[T any](capacity int) *RingBuffer[T] {
	if capacity <= 0 {
		capacity = 60
	}
	return &RingBuffer[T]{
		data: make([]T, capacity),
		cap:  capacity,
	}
}

// Push adds an item to the buffer. If the buffer is full, the oldest item
// is overwritten.
func (rb *RingBuffer[T]) Push(item T) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	idx := (rb.head + rb.size) % rb.cap
	rb.data[idx] = item

	if rb.size < rb.cap {
		rb.size++
	} else {
		rb.head = (rb.head + 1) % rb.cap
	}
}

// Drain returns all items in order (oldest first) and clears the buffer.
func (rb *RingBuffer[T]) Drain() []T {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.size == 0 {
		return nil
	}

	out := make([]T, rb.size)
	for i := 0; i < rb.size; i++ {
		out[i] = rb.data[(rb.head+i)%rb.cap]
	}

	rb.head = 0
	rb.size = 0
	return out
}

// Len returns the number of items in the buffer.
func (rb *RingBuffer[T]) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.size
}
