package resources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
)

// maxAgentDownloadBytes is the browser download limit for agent files.
const maxAgentDownloadBytes = 512 * 1024 * 1024

func (d *Deps) HandleFileDownload(w http.ResponseWriter, r *http.Request, assetID string) {
	d.HandleFileDownloadWithTimeout(w, r, assetID, fileRequestTimeout)
}

// HandleFileDownloadWithTimeout sends the file while it arrives from the
// agent, so a large download starts at once instead of after the whole file
// is staged (the console's proxy gave up after 5 minutes without headers).
func (d *Deps) HandleFileDownloadWithTimeout(w http.ResponseWriter, r *http.Request, assetID string, timeout time.Duration) {
	d.handleAgentFileDownload(w, r, assetID, timeout, maxAgentDownloadBytes, true)
}

// HandleFileDownloadWithLimit performs the normal correlated agent download
// flow while allowing trusted internal callers (such as MCP) to impose a much
// smaller response limit than the browser download endpoint. The file is
// staged first, so the caller gets either the whole file or an error status.
func (d *Deps) HandleFileDownloadWithLimit(w http.ResponseWriter, r *http.Request, assetID string, timeout time.Duration, maxDownloadBytes int64) {
	d.handleAgentFileDownload(w, r, assetID, timeout, maxDownloadBytes, false)
}

func (d *Deps) handleAgentFileDownload(w http.ResponseWriter, r *http.Request, assetID string, timeout time.Duration, maxDownloadBytes int64, stream bool) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if r.Context().Err() != nil {
		return
	}

	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "path is required")
		return
	}
	deadline := newFileStreamDeadline(w)

	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent disconnected")
		return
	}

	requestID := generateRequestID()

	bridge := newFileBridge(64, assetID)
	d.FileBridges.Store(requestID, bridge)
	defer bridge.Close()
	defer d.FileBridges.Delete(requestID)

	data, _ := json.Marshal(agentmgr.FileReadData{
		RequestID: requestID,
		Path:      filePath,
	})
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgFileRead,
		ID:   requestID,
		Data: data,
	}); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to send request to agent")
		return
	}

	firstChunk, err := receiveFileDownloadChunk(r.Context(), bridge, timeout)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		if errors.Is(err, errFileDownloadTimedOut) {
			servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
			return
		}
		if errors.Is(err, errFileDownloadBackpressured) {
			servicehttp.WriteError(w, http.StatusBadGateway, err.Error())
			return
		}
		if errors.Is(err, errFileAgentDisconnected) {
			writeFileBridgeClosedError(w, bridge)
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
		return
	}
	if firstChunk.Error != "" {
		servicehttp.WriteError(w, http.StatusBadRequest, firstChunk.Error)
		return
	}
	firstPayload, err := DecodeFileDownloadChunk(firstChunk)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
		return
	}
	if firstChunk.Offset != 0 {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent sent invalid file chunk offset")
		return
	}

	if int64(len(firstPayload)) > maxDownloadBytes {
		servicehttp.WriteError(w, http.StatusRequestEntityTooLarge, "agent download exceeded byte limit")
		return
	}

	// The agent protocol has no flow control, so every chunk goes to a spool
	// at the agent's pace; a slow browser must not stall the agent's stream.
	spool, err := d.createFileSpool("download-*")
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to prepare download")
		return
	}
	defer func() {
		if err := spool.Close(); err != nil {
			securityruntime.Logf("file: failed to close download spool for %s: %v", filePath, err)
		}
		if err := os.Remove(spool.Name()); err != nil {
			securityruntime.Logf("file: failed to remove download spool for %s: %v", filePath, err)
		}
	}()

	spoolChunks := func(ctx context.Context, dst io.Writer) (int64, error) {
		return spoolFileDownloadChunks(ctx, dst, bridge, filePath, timeout, firstChunk, firstPayload, maxDownloadBytes)
	}
	if stream {
		streamFileDownload(w, r, spool, deadline, filePath, spoolChunks)
		return
	}
	sendStagedFileDownload(w, r, spool, deadline, filePath, spoolChunks)
}

func writeFileDownloadHeaders(w http.ResponseWriter, filePath string) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": remoteFileBase(filePath)}))
}

// sendStagedFileDownload receives the whole file, then sends it with its
// length. Any failure is reported with an error status and no file data.
func sendStagedFileDownload(w http.ResponseWriter, r *http.Request, spool *os.File, deadline fileStreamDeadline, filePath string, spoolChunks func(context.Context, io.Writer) (int64, error)) {
	written, err := spoolChunks(r.Context(), progressWriter{Writer: spool, deadline: deadline})
	if r.Context().Err() != nil {
		return
	}
	if err != nil {
		writeFileDownloadError(w, err)
		return
	}
	if _, err := spool.Seek(0, io.SeekStart); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to prepare download")
		return
	}
	writeFileDownloadHeaders(w, filePath)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", written))
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(progressWriter{Writer: w, deadline: deadline}, spool); err != nil {
		securityruntime.Logf("file: failed to write download response for %s: %v", filePath, err)
	}
}
