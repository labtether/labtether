package terminal

import (
	"bytes"
	"sync"
)

// RingBuffer is a fixed-capacity circular line buffer for terminal output.
// Thread-safe via mutex. Tracks line boundaries by newline characters.
type RingBuffer struct {
	mu       sync.Mutex
	lines    [][]byte // circular array of complete lines
	maxLines int
	head     int    // next write position
	count    int    // current number of lines stored
	partial  []byte // incomplete line (no trailing newline yet)
}

func NewRingBuffer(maxLines int) *RingBuffer {
	return &RingBuffer{
		lines:    make([][]byte, maxLines),
		maxLines: maxLines,
	}
}

func (rb *RingBuffer) Write(p []byte) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	data := p
	if len(rb.partial) > 0 {
		data = append(rb.partial, p...)
		rb.partial = nil
	}

	for len(data) > 0 {
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			rb.partial = append([]byte{}, data...)
			return
		}
		line := make([]byte, idx+1)
		copy(line, data[:idx+1])
		rb.pushLine(line)
		data = data[idx+1:]
	}
}

func (rb *RingBuffer) pushLine(line []byte) {
	rb.lines[rb.head] = line
	rb.head = (rb.head + 1) % rb.maxLines
	if rb.count < rb.maxLines {
		rb.count++
	}
}

func (rb *RingBuffer) Snapshot() []byte {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.count == 0 && len(rb.partial) == 0 {
		return nil
	}

	var buf bytes.Buffer
	start := 0
	if rb.count == rb.maxLines {
		start = rb.head // oldest line when buffer is full
	}
	for i := 0; i < rb.count; i++ {
		idx := (start + i) % rb.maxLines
		buf.Write(rb.lines[idx])
	}
	if len(rb.partial) > 0 {
		buf.Write(rb.partial)
	}
	return buf.Bytes()
}

func (rb *RingBuffer) Lines() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count
}

func (rb *RingBuffer) ByteSize() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	size := len(rb.partial)
	start := 0
	if rb.count == rb.maxLines {
		start = rb.head
	}
	for i := 0; i < rb.count; i++ {
		idx := (start + i) % rb.maxLines
		size += len(rb.lines[idx])
	}
	return size
}
