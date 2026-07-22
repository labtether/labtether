package fileproto

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestListingLimiterVisibleEntryLimit(t *testing.T) {
	limiter := newListingLimiter(true)
	entries := make([]FileEntry, 0, MaxVisibleListEntries)
	for i := 0; i < MaxVisibleListEntries; i++ {
		var err error
		entries, err = limiter.append(entries, FileEntry{Name: "x", Path: "/x"})
		if err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
	}
	if _, err := limiter.append(entries, FileEntry{Name: "overflow", Path: "/overflow"}); !errors.Is(err, ErrListLimitExceeded) {
		t.Fatalf("expected ErrListLimitExceeded, got %v", err)
	}
}

func TestListingLimiterCountsHiddenScannedEntries(t *testing.T) {
	limiter := newListingLimiter(false)
	var entries []FileEntry
	for i := 0; i < MaxScannedListEntries; i++ {
		var err error
		entries, err = limiter.append(entries, FileEntry{Name: ".hidden", Path: "/.hidden"})
		if err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
	}
	if len(entries) != 0 {
		t.Fatalf("hidden entries unexpectedly visible: %d", len(entries))
	}
	if _, err := limiter.append(entries, FileEntry{Name: ".overflow", Path: "/.overflow"}); !errors.Is(err, ErrListLimitExceeded) {
		t.Fatalf("expected ErrListLimitExceeded, got %v", err)
	}
}

func TestListingLimiterResponseByteLimit(t *testing.T) {
	limiter := newListingLimiter(true)
	huge := strings.Repeat("x", MaxListResponseBytes)
	if _, err := limiter.append(nil, FileEntry{Name: huge, Path: "/x"}); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("expected ErrResponseTooLarge, got %v", err)
	}
}

func TestBoundedReaderRejectsOnlyBytesBeyondLimit(t *testing.T) {
	exact := newBoundedReader(context.Background(), bytes.NewReader([]byte("four")), 4, ErrTransferTooLarge)
	got, err := io.ReadAll(exact)
	if err != nil {
		t.Fatalf("exact-limit read failed: %v", err)
	}
	if string(got) != "four" {
		t.Fatalf("read %q, want four", got)
	}

	over := newBoundedReader(context.Background(), bytes.NewReader([]byte("five!")), 4, ErrTransferTooLarge)
	got, err = io.ReadAll(over)
	if !errors.Is(err, ErrTransferTooLarge) {
		t.Fatalf("expected ErrTransferTooLarge, got data=%q err=%v", got, err)
	}
	if string(got) != "five" {
		t.Fatalf("read %q before limit error, want five", got)
	}
}

func TestWithOperationTimeoutHonorsEarlierCallerDeadline(t *testing.T) {
	caller, callerCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer callerCancel()
	op, cancel := WithOperationTimeout(caller)
	defer cancel()
	deadline, ok := op.Deadline()
	if !ok {
		t.Fatal("operation context has no deadline")
	}
	if remaining := time.Until(deadline); remaining > 100*time.Millisecond {
		t.Fatalf("operation ignored earlier caller deadline: %v", remaining)
	}
}

func TestDeleteBudgetLimitsDepthAndEntries(t *testing.T) {
	if err := (&deleteBudget{}).enter(MaxRecursiveDeleteDepth+1, 0); !errors.Is(err, ErrDeleteLimitExceeded) {
		t.Fatalf("expected depth limit error, got %v", err)
	}
	b := &deleteBudget{}
	if err := b.enter(0, MaxRecursiveDeleteItems); err != nil {
		t.Fatalf("exact entry limit failed: %v", err)
	}
	if err := b.enter(0, 1); !errors.Is(err, ErrDeleteLimitExceeded) {
		t.Fatalf("expected entry limit error, got %v", err)
	}
}

func TestFTPDataReadBudgetRejectsBytesAcrossConnections(t *testing.T) {
	budget := newFTPDataReadBudget(4, ErrResponseTooLarge)
	first := strings.NewReader("abc")
	second := strings.NewReader("de")

	got, err := io.ReadAll(&ftpBudgetTestReader{budget: budget, source: first})
	if err != nil || string(got) != "abc" {
		t.Fatalf("first connection read data=%q err=%v", got, err)
	}
	got, err = io.ReadAll(&ftpBudgetTestReader{budget: budget, source: second})
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("expected shared ErrResponseTooLarge, got data=%q err=%v", got, err)
	}
	if string(got) != "d" {
		t.Fatalf("second connection data=%q, want one remaining byte", got)
	}
}

type ftpBudgetTestReader struct {
	budget *ftpDataReadBudget
	source io.Reader
}

func (r *ftpBudgetTestReader) Read(p []byte) (int, error) {
	return r.budget.read(r.source, p)
}

func TestProtocolClientsBoundAndCloseRawConnections(t *testing.T) {
	for _, tc := range []struct {
		name   string
		client RemoteFS
		conn   *deadlineRecordingConn
	}{
		{
			name: "sftp",
			conn: &deadlineRecordingConn{},
		},
		{
			name: "smb",
			conn: &deadlineRecordingConn{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			switch tc.name {
			case "sftp":
				tc.client = &SFTPClient{rawConn: tc.conn}
			case "smb":
				tc.client = &SMBClient{conn: tc.conn}
			}
			started := time.Now()
			if err := tc.client.Close(); err != nil {
				t.Fatalf("close: %v", err)
			}
			if !tc.conn.closed {
				t.Fatal("raw connection was not closed")
			}
			minimum := started.Add(protocolCloseTimeout - time.Second)
			maximum := started.Add(protocolCloseTimeout + time.Second)
			if tc.conn.deadline.Before(minimum) || tc.conn.deadline.After(maximum) {
				t.Fatalf("close deadline=%v, want between %v and %v", tc.conn.deadline, minimum, maximum)
			}
		})
	}
}

type deadlineRecordingConn struct {
	deadline time.Time
	closed   bool
}

func (c *deadlineRecordingConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *deadlineRecordingConn) Write(p []byte) (int, error)      { return len(p), nil }
func (c *deadlineRecordingConn) Close() error                     { c.closed = true; return nil }
func (c *deadlineRecordingConn) LocalAddr() net.Addr              { return deadlineRecordingAddr("local") }
func (c *deadlineRecordingConn) RemoteAddr() net.Addr             { return deadlineRecordingAddr("remote") }
func (c *deadlineRecordingConn) SetDeadline(t time.Time) error    { c.deadline = t; return nil }
func (c *deadlineRecordingConn) SetReadDeadline(time.Time) error  { return nil }
func (c *deadlineRecordingConn) SetWriteDeadline(time.Time) error { return nil }

type deadlineRecordingAddr string

func (a deadlineRecordingAddr) Network() string { return "test" }
func (a deadlineRecordingAddr) String() string  { return string(a) }
