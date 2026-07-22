package fileproto

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	MaxVisibleListEntries         = 5_000
	MaxScannedListEntries         = 20_000
	MaxListResponseBytes          = 4 * 1024 * 1024
	MaxTransferBytes        int64 = 512 * 1024 * 1024
	MaxOperationDuration          = 5 * time.Minute
	MaxRecursiveDeleteDepth       = 64
	MaxRecursiveDeleteItems       = 20_000

	fileCopyBufferSize   = 64 * 1024
	protocolCloseTimeout = 10 * time.Second
)

func setProtocolCloseDeadline(conn net.Conn) {
	if conn != nil {
		_ = conn.SetDeadline(time.Now().Add(protocolCloseTimeout))
	}
}

var (
	ErrListLimitExceeded   = errors.New("directory listing exceeds limit")
	ErrResponseTooLarge    = errors.New("directory listing response exceeds limit")
	ErrTransferTooLarge    = errors.New("file exceeds 512 MB limit")
	ErrDeleteLimitExceeded = errors.New("recursive delete exceeds limit")
)

// WithOperationTimeout caps remote operations at five minutes while retaining
// any earlier caller deadline or cancellation.
func WithOperationTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, MaxOperationDuration)
}

type listingLimiter struct {
	showHidden   bool
	scanned      int
	visible      int
	encodedBytes int
}

func newListingLimiter(showHidden bool) *listingLimiter {
	return &listingLimiter{showHidden: showHidden, encodedBytes: 2} // []
}

func (l *listingLimiter) append(result []FileEntry, entry FileEntry) ([]FileEntry, error) {
	l.scanned++
	if l.scanned > MaxScannedListEntries {
		return nil, ErrListLimitExceeded
	}
	if !l.showHidden && strings.HasPrefix(entry.Name, ".") {
		return result, nil
	}
	if l.visible >= MaxVisibleListEntries {
		return nil, ErrListLimitExceeded
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return nil, err
	}
	projected := l.encodedBytes + len(encoded)
	if l.visible > 0 {
		projected++ // comma
	}
	if projected > MaxListResponseBytes {
		return nil, ErrResponseTooLarge
	}
	l.encodedBytes = projected
	l.visible++
	return append(result, entry), nil
}

type boundedReader struct {
	ctx      context.Context
	r        io.Reader
	limit    int64
	read     int64
	limitErr error
	lastErr  error
}

func newBoundedReader(ctx context.Context, r io.Reader, limit int64, limitErr error) *boundedReader {
	return &boundedReader{ctx: ctx, r: r, limit: limit, limitErr: limitErr}
}

func (r *boundedReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		r.lastErr = err
		return 0, err
	}
	if r.read >= r.limit {
		var probe [1]byte
		n, err := r.r.Read(probe[:])
		if n > 0 {
			r.lastErr = r.limitErr
			return 0, r.limitErr
		}
		if ctxErr := r.ctx.Err(); ctxErr != nil {
			r.lastErr = ctxErr
			return 0, ctxErr
		}
		if err != nil {
			r.lastErr = err
		}
		return 0, err
	}
	remaining := r.limit - r.read
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}
	n, err := r.r.Read(p)
	r.read += int64(n)
	if ctxErr := r.ctx.Err(); ctxErr != nil {
		r.lastErr = ctxErr
		return n, ctxErr
	}
	if err != nil {
		r.lastErr = err
	}
	return n, err
}

func (r *boundedReader) terminalError() error {
	if r.lastErr == nil || errors.Is(r.lastErr, io.EOF) {
		return nil
	}
	return r.lastErr
}

type boundedOperationReadCloser struct {
	reader  *boundedReader
	source  io.ReadCloser
	cleanup func()
	done    chan struct{}
	once    sync.Once
	err     error
}

func newBoundedOperationReadCloser(ctx context.Context, source io.ReadCloser, limit int64, limitErr error, cleanup func()) io.ReadCloser {
	r := &boundedOperationReadCloser{
		reader:  newBoundedReader(ctx, source, limit, limitErr),
		source:  source,
		cleanup: cleanup,
		done:    make(chan struct{}),
	}
	go func() {
		select {
		case <-ctx.Done():
			_ = r.Close()
		case <-r.done:
		}
	}()
	return r
}

func (r *boundedOperationReadCloser) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r *boundedOperationReadCloser) Close() error {
	r.once.Do(func() {
		close(r.done)
		r.err = r.source.Close()
		if r.cleanup != nil {
			r.cleanup()
		}
	})
	return r.err
}

func validateTransferSize(size int64) error {
	if size > MaxTransferBytes {
		return ErrTransferTooLarge
	}
	return nil
}

func beginNetConnOperation(parent context.Context, mu *sync.Mutex, conn net.Conn) (context.Context, func(), error) {
	mu.Lock()
	ctx, cancel := WithOperationTimeout(parent)
	if err := ctx.Err(); err != nil {
		cancel()
		mu.Unlock()
		return nil, nil, err
	}
	deadline, _ := ctx.Deadline()
	if conn != nil {
		if err := conn.SetDeadline(deadline); err != nil {
			cancel()
			mu.Unlock()
			return nil, nil, err
		}
	}
	stopDeadline := make(chan struct{})
	deadlineStopped := make(chan struct{})
	go func() {
		defer close(deadlineStopped)
		select {
		case <-ctx.Done():
			if conn != nil {
				_ = conn.SetDeadline(time.Now())
			}
		case <-stopDeadline:
		}
	}()
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			close(stopDeadline)
			cancel()
			// Wait until a concurrent deadline update is finished before clearing
			// it for the next serialized operation.
			<-deadlineStopped
			if conn != nil {
				_ = conn.SetDeadline(time.Time{})
			}
			mu.Unlock()
		})
	}
	return ctx, cleanup, nil
}

// watchConnCancellation interrupts blocked network I/O as soon as ctx is
// cancelled. The returned function is idempotent and waits for a concurrent
// deadline update to finish, allowing callers to safely clear the deadline.
func watchConnCancellation(ctx context.Context, conn net.Conn) func() {
	stop := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			_ = conn.SetDeadline(time.Now())
		case <-stop:
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(stop)
			<-stopped
		})
	}
}

type deleteBudget struct {
	entries int
}

func (b *deleteBudget) enter(depth int, count int) error {
	if depth > MaxRecursiveDeleteDepth || count < 0 || b.entries > MaxRecursiveDeleteItems-count {
		return ErrDeleteLimitExceeded
	}
	b.entries += count
	return nil
}
