package fileproto

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
)

// transferMockFS is a minimal in-memory RemoteFS for transfer tests.
type transferMockFS struct {
	mu       sync.Mutex
	files    map[string][]byte
	readErr  error // if set, Read returns this error
	writeErr error // if set, Write returns this error
}

func newTransferMockFS() *transferMockFS {
	return &transferMockFS{files: make(map[string][]byte)}
}

func (m *transferMockFS) Connect(_ context.Context, _ ConnectionConfig) error { return nil }
func (m *transferMockFS) List(_ context.Context, _ string, _ bool) ([]FileEntry, error) {
	return nil, nil
}
func (m *transferMockFS) Mkdir(_ context.Context, _ string) error        { return nil }
func (m *transferMockFS) Delete(_ context.Context, _ string) error       { return nil }
func (m *transferMockFS) Rename(_ context.Context, _, _ string) error    { return nil }
func (m *transferMockFS) Copy(_ context.Context, _, _ string) error      { return nil }
func (m *transferMockFS) Close() error                                   { return nil }

func (m *transferMockFS) Read(_ context.Context, path string) (io.ReadCloser, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.readErr != nil {
		return nil, 0, m.readErr
	}
	data, ok := m.files[path]
	if !ok {
		return nil, 0, errors.New("file not found")
	}
	return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
}

func (m *transferMockFS) Write(_ context.Context, path string, r io.Reader, _ int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return m.writeErr
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.files[path] = data
	return nil
}

func (m *transferMockFS) getFile(path string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.files[path]
	return data, ok
}

func (m *transferMockFS) putFile(path string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[path] = data
}

func TestTransfer_Success(t *testing.T) {
	src := newTransferMockFS()
	dst := newTransferMockFS()

	payload := bytes.Repeat([]byte("hello world "), 1000) // ~12 KB
	src.putFile("/docs/readme.txt", payload)

	ctx := context.Background()
	n, err := Transfer(ctx, src, "/docs/readme.txt", dst, "/backup/readme.txt", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != int64(len(payload)) {
		t.Fatalf("expected %d bytes transferred, got %d", len(payload), n)
	}

	got, ok := dst.getFile("/backup/readme.txt")
	if !ok {
		t.Fatal("destination file not written")
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("destination content does not match source")
	}
}

func TestTransfer_ProgressCallback(t *testing.T) {
	src := newTransferMockFS()
	dst := newTransferMockFS()

	payload := make([]byte, 256*1024) // 256 KB — multiple read chunks
	for i := range payload {
		payload[i] = byte(i % 251) // fill with non-zero data
	}
	src.putFile("/big.bin", payload)

	var mu sync.Mutex
	var calls []struct{ transferred, total int64 }

	progress := func(transferred, total int64) {
		mu.Lock()
		calls = append(calls, struct{ transferred, total int64 }{transferred, total})
		mu.Unlock()
	}

	ctx := context.Background()
	n, err := Transfer(ctx, src, "/big.bin", dst, "/copy.bin", progress)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != int64(len(payload)) {
		t.Fatalf("expected %d bytes, got %d", len(payload), n)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(calls) == 0 {
		t.Fatal("expected at least one progress callback, got none")
	}

	// Every call should report the correct total.
	for i, c := range calls {
		if c.total != int64(len(payload)) {
			t.Fatalf("call %d: expected total=%d, got %d", i, len(payload), c.total)
		}
	}

	// transferred values should be monotonically increasing.
	for i := 1; i < len(calls); i++ {
		if calls[i].transferred <= calls[i-1].transferred {
			t.Fatalf("progress not monotonically increasing at call %d: %d <= %d",
				i, calls[i].transferred, calls[i-1].transferred)
		}
	}

	// Final callback should report full transfer.
	last := calls[len(calls)-1]
	if last.transferred != int64(len(payload)) {
		t.Fatalf("final progress: expected %d transferred, got %d", len(payload), last.transferred)
	}
}

func TestTransfer_CancelledContext(t *testing.T) {
	src := newTransferMockFS()
	// Use a special destination that reads all data from the reader (so the
	// progressReader's ctx check is exercised).
	dst := newTransferMockFS()

	payload := make([]byte, 512*1024) // 512 KB
	src.putFile("/data.bin", payload)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a tiny amount of progress.
	progress := func(transferred, _ int64) {
		if transferred > 1024 {
			cancel()
		}
	}

	_, err := Transfer(ctx, src, "/data.bin", dst, "/out.bin", progress)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	// The error chain should contain the context cancellation.
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled in error chain, got: %v", err)
	}
}

func TestTransfer_SourceReadError(t *testing.T) {
	src := newTransferMockFS()
	src.readErr = errors.New("permission denied")
	dst := newTransferMockFS()

	ctx := context.Background()
	_, err := Transfer(ctx, src, "/secret.bin", dst, "/out.bin", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, src.readErr) {
		t.Fatalf("expected wrapped readErr, got: %v", err)
	}
}

func TestTransfer_DestWriteError(t *testing.T) {
	src := newTransferMockFS()
	src.putFile("/data.bin", []byte("content"))

	dst := newTransferMockFS()
	dst.writeErr = errors.New("disk full")

	ctx := context.Background()
	_, err := Transfer(ctx, src, "/data.bin", dst, "/out.bin", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, dst.writeErr) {
		t.Fatalf("expected wrapped writeErr, got: %v", err)
	}
}
