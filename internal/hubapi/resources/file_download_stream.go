package resources

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"sync"

	"github.com/labtether/labtether/internal/securityruntime"
)

const fileDownloadStreamBufferBytes = 32 * 1024

// spoolProgress records how much of a download has reached its spool, so the
// response can read the spool while the agent's chunks are still arriving.
type spoolProgress struct {
	mu      sync.Mutex
	written int64
	done    bool
	err     error
	changed chan struct{}
}

func newSpoolProgress() *spoolProgress {
	return &spoolProgress{changed: make(chan struct{}, 1)}
}

func (p *spoolProgress) add(n int64) {
	p.mu.Lock()
	p.written += n
	p.mu.Unlock()
	p.signal()
}

func (p *spoolProgress) finish(err error) {
	p.mu.Lock()
	p.done = true
	p.err = err
	p.mu.Unlock()
	p.signal()
}

func (p *spoolProgress) signal() {
	select {
	case p.changed <- struct{}{}:
	default:
	}
}

func (p *spoolProgress) snapshot() (int64, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.written, p.done, p.err
}

type progressSpoolWriter struct {
	file     *os.File
	progress *spoolProgress
}

func (w progressSpoolWriter) Write(p []byte) (int, error) {
	n, err := w.file.Write(p)
	w.progress.add(int64(n))
	return n, err
}

// streamFileDownload spools the agent's chunks in the background and sends
// each byte to the browser as soon as it is in the spool. Headers go out at
// once. A failure after that point aborts the connection, so the browser
// reports a failed download instead of keeping a truncated file.
func streamFileDownload(w http.ResponseWriter, r *http.Request, spool *os.File, deadline fileStreamDeadline, filePath string, spoolChunks func(context.Context, io.Writer) (int64, error)) {
	ctx, cancel := context.WithCancel(r.Context())
	progress := newSpoolProgress()
	spooled := make(chan struct{})
	go func() {
		defer close(spooled)
		_, err := spoolChunks(ctx, progressSpoolWriter{file: spool, progress: progress})
		progress.finish(err)
	}()
	// Runs before the caller removes the spool and closes the agent bridge,
	// including when the response is aborted.
	defer func() {
		cancel()
		<-spooled
	}()

	writeFileDownloadHeaders(w, filePath)
	w.WriteHeader(http.StatusOK)
	out := progressWriter{Writer: w, deadline: deadline}
	controller := http.NewResponseController(w)
	buf := make([]byte, fileDownloadStreamBufferBytes)
	var sent int64
	for {
		written, done, spoolErr := progress.snapshot()
		if sent < written {
			n := min(int64(len(buf)), written-sent)
			read, err := spool.ReadAt(buf[:n], sent)
			if read > 0 {
				if _, writeErr := out.Write(buf[:read]); writeErr != nil {
					securityruntime.Logf("file: failed to write download response for %s: %v", filePath, writeErr)
					return
				}
				sent += int64(read)
			}
			if err != nil && (read == 0 || !errors.Is(err, io.EOF)) {
				securityruntime.Logf("file: failed to read download spool for %s: %v", filePath, err)
				panic(http.ErrAbortHandler)
			}
			continue
		}
		if done {
			if spoolErr != nil && r.Context().Err() == nil {
				securityruntime.Logf("file: download for %s failed after it started: %v", filePath, spoolErr)
				panic(http.ErrAbortHandler)
			}
			return
		}
		_ = controller.Flush()
		select {
		case <-progress.changed:
		case <-r.Context().Done():
			return
		}
	}
}
