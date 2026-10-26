package terminal

import (
	"errors"
	"io"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestSSHStalledInputClosesOwnedClient(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	var closed atomic.Bool
	_, err := handleSSHInput(nil, writer, []byte("stalled paste"), func() error {
		closed.Store(true)
		return writer.Close()
	}, 20*time.Millisecond)
	if !errors.Is(err, os.ErrDeadlineExceeded) || !closed.Load() {
		t.Fatalf("stalled input did not close its client: err=%v closed=%v", err, closed.Load())
	}
}

func TestSSHInputPreservesBytesWithoutClosingClient(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	data := []byte("  paste\r\n\x1b[200~😀\x1b[201~")
	got := make(chan []byte, 1)
	go func() {
		buf := make([]byte, len(data))
		_, _ = io.ReadFull(reader, buf)
		got <- buf
	}()
	var closed atomic.Bool
	_, err := handleSSHInput(nil, writer, data, func() error {
		closed.Store(true)
		return writer.Close()
	}, time.Second)
	if err != nil || closed.Load() {
		t.Fatalf("successful input lost its client: err=%v closed=%v", err, closed.Load())
	}
	if received := <-got; string(received) != string(data) {
		t.Fatalf("stdin bytes changed: %q", received)
	}
}
