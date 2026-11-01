package resources

import (
	"io"
	"net/http"
	"time"
)

// fileStreamIdleTimeout bounds how long an upload or download may go without
// moving any bytes. The Hub's server-wide read and write timeouts are sized for
// API calls, so without this a large transfer is cut off partway through.
const fileStreamIdleTimeout = 2 * time.Minute

// fileStreamDeadline pushes the connection's read and write deadlines forward
// while a transfer makes progress, so only a stalled transfer times out.
type fileStreamDeadline struct {
	controller *http.ResponseController
}

func newFileStreamDeadline(w http.ResponseWriter) fileStreamDeadline {
	deadline := fileStreamDeadline{controller: http.NewResponseController(w)}
	deadline.extend()
	return deadline
}

// extend is best effort. Writers without deadline support, such as test
// recorders and the internal MCP recorder, have no server timeouts to extend.
func (d fileStreamDeadline) extend() {
	if d.controller == nil {
		return
	}
	at := time.Now().Add(fileStreamIdleTimeout)
	_ = d.controller.SetReadDeadline(at)
	_ = d.controller.SetWriteDeadline(at)
}

// progressBody extends the transfer deadlines before every read of an upload
// body, so a slow but steady upload is never cut off.
type progressBody struct {
	io.ReadCloser
	deadline fileStreamDeadline
}

func (b progressBody) Read(p []byte) (int, error) {
	b.deadline.extend()
	return b.ReadCloser.Read(p)
}

// progressWriter extends the transfer deadlines before every write of a
// download response, so a slow but steady download is never cut off.
type progressWriter struct {
	io.Writer
	deadline fileStreamDeadline
}

func (w progressWriter) Write(p []byte) (int, error) {
	w.deadline.extend()
	return w.Writer.Write(p)
}
