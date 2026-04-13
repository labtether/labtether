package resources

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
)

var errFileDownloadTimedOut = errors.New("file download timed out")
var errFileDownloadBackpressured = errors.New("file download could not keep up with the agent stream")

func (d *Deps) HandleFileList(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	dirPath := strings.TrimSpace(r.URL.Query().Get("path"))
	showHidden := r.URL.Query().Get("show_hidden") == "true"

	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent disconnected")
		return
	}

	requestID := generateRequestID()

	// Set up response channel.
	bridge := newFileBridge(1, assetID)
	d.FileBridges.Store(requestID, bridge)
	defer bridge.Close()
	defer d.FileBridges.Delete(requestID)

	data, _ := json.Marshal(agentmgr.FileListData{
		RequestID:  requestID,
		Path:       dirPath,
		ShowHidden: showHidden,
	})
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgFileList,
		ID:   requestID,
		Data: data,
	}); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to send request to agent")
		return
	}

	select {
	case msg := <-bridge.Ch:
		var listed agentmgr.FileListedData
		if err := json.Unmarshal(msg.Data, &listed); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
			return
		}
		if listed.Error != "" {
			servicehttp.WriteError(w, http.StatusBadRequest, listed.Error)
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, listed)
	case <-time.After(fileRequestTimeout):
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
	}
}

// handleFileDownload streams a file from the agent to the browser.
func (d *Deps) HandleFileDownload(w http.ResponseWriter, r *http.Request, assetID string) {
	d.HandleFileDownloadWithTimeout(w, r, assetID, fileRequestTimeout)
}

func (d *Deps) HandleFileDownloadWithTimeout(w http.ResponseWriter, r *http.Request, assetID string, timeout time.Duration) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	filePath := strings.TrimSpace(r.URL.Query().Get("path"))
	if filePath == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "path is required")
		return
	}

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

	tempFile, err := os.CreateTemp("", "labtether-download-*")
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to prepare download buffer")
		return
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
	}()

	firstChunk, err := receiveFileDownloadChunk(bridge, timeout)
	if err != nil {
		if errors.Is(err, errFileDownloadTimedOut) {
			servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
			return
		}
		if errors.Is(err, errFileDownloadBackpressured) {
			servicehttp.WriteError(w, http.StatusBadGateway, err.Error())
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

	if len(firstPayload) > 0 {
		if _, err := tempFile.Write(firstPayload); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to buffer download")
			return
		}
	}
	if err := bufferRemainingDownloadChunks(tempFile, bridge, filePath, timeout, firstChunk.Done); err != nil {
		switch {
		case errors.Is(err, errFileDownloadTimedOut):
			servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
		case errors.Is(err, errFileDownloadBackpressured):
			servicehttp.WriteError(w, http.StatusBadGateway, err.Error())
		case errors.Is(err, errFileDownloadAgentFailed):
			servicehttp.WriteError(w, http.StatusBadGateway, err.Error())
		default:
			servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
		}
		return
	}
	if _, err := tempFile.Seek(0, io.SeekStart); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to finalize download")
		return
	}

	parts := strings.Split(filePath, "/")
	filename := parts[len(parts)-1]
	if filename == "" {
		filename = "download"
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, tempFile); err != nil {
		securityruntime.Logf("file: failed to write download response for %s: %v", filePath, err)
	}
}

func receiveFileDownloadChunk(bridge *FileBridge, timeout time.Duration) (agentmgr.FileDataPayload, error) {
	if timeout <= 0 {
		timeout = fileRequestTimeout
	}
	select {
	case msg := <-bridge.Ch:
		var chunk agentmgr.FileDataPayload
		if err := json.Unmarshal(msg.Data, &chunk); err != nil {
			return agentmgr.FileDataPayload{}, err
		}
		return chunk, nil
	case <-bridge.Done:
		if err := bridge.Err(); err != nil {
			if errors.Is(err, errFileResponseBackpressured) {
				return agentmgr.FileDataPayload{}, errFileDownloadBackpressured
			}
			return agentmgr.FileDataPayload{}, err
		}
		return agentmgr.FileDataPayload{}, errFileDownloadTimedOut
	case <-time.After(timeout):
		return agentmgr.FileDataPayload{}, errFileDownloadTimedOut
	}
}

func DecodeFileDownloadChunk(chunk agentmgr.FileDataPayload) ([]byte, error) {
	if chunk.Data == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(chunk.Data)
}

var errFileDownloadAgentFailed = errors.New("agent reported a file download failure")

func bufferRemainingDownloadChunks(dst *os.File, bridge *FileBridge, filePath string, timeout time.Duration, firstChunkDone bool) error {
	if firstChunkDone {
		return nil
	}
	for {
		chunk, err := receiveFileDownloadChunk(bridge, timeout)
		if err != nil {
			if errors.Is(err, errFileDownloadTimedOut) {
				securityruntime.Logf("file: download timed out for %s", filePath)
			}
			if errors.Is(err, errFileDownloadBackpressured) {
				securityruntime.Logf("file: download backpressure exceeded for %s", filePath)
			}
			return err
		}
		if chunk.Error != "" {
			securityruntime.Logf("file: download error for %s: %s", filePath, chunk.Error)
			return fmt.Errorf("%w: %s", errFileDownloadAgentFailed, chunk.Error)
		}
		payload, err := DecodeFileDownloadChunk(chunk)
		if err != nil {
			return err
		}
		if len(payload) > 0 {
			if _, err := dst.Write(payload); err != nil {
				return err
			}
		}
		if chunk.Done {
			return nil
		}
	}
}

// handleFileUpload receives a file from the browser and relays to the agent.
func (d *Deps) HandleFileUpload(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	filePath := strings.TrimSpace(r.URL.Query().Get("path"))
	if filePath == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "path is required")
		return
	}

	// Enforce 512MB upload limit.
	const maxUploadBytes = 512 * 1024 * 1024
	if r.ContentLength > maxUploadBytes {
		servicehttp.WriteError(w, http.StatusRequestEntityTooLarge, "file exceeds 512 MB limit")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent disconnected")
		return
	}

	requestID := generateRequestID()

	bridge := newFileBridge(1, assetID)
	d.FileBridges.Store(requestID, bridge)
	defer bridge.Close()
	defer d.FileBridges.Delete(requestID)

	if _, err := RelayFileUploadChunks(r.Body, requestID, filePath, fileChunkSizeHub, func(payload agentmgr.FileWriteData) error {
		data, _ := json.Marshal(payload)
		return agentConn.Send(agentmgr.Message{
			Type: agentmgr.MsgFileWrite,
			ID:   requestID,
			Data: data,
		})
	}); err != nil {
		securityruntime.Logf("file: upload relay error for %s: %v", filePath, err)
		var sendErr UploadRelaySendError
		if errors.As(err, &sendErr) {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to relay data to agent")
		} else {
			servicehttp.WriteError(w, http.StatusInternalServerError, "upload failed")
		}
		return
	}

	// Wait for write confirmation.
	select {
	case msg := <-bridge.Ch:
		var result agentmgr.FileWrittenData
		if err := json.Unmarshal(msg.Data, &result); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
			return
		}
		if result.Error != "" {
			servicehttp.WriteError(w, http.StatusBadRequest, result.Error)
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, result)
	case <-time.After(fileRequestTimeout):
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
	}
}

func RelayFileUploadChunks(
	body io.Reader,
	requestID, filePath string,
	chunkSize int,
	send func(agentmgr.FileWriteData) error,
) (int64, error) {
	if body == nil {
		return 0, io.EOF
	}
	if send == nil {
		return 0, fmt.Errorf("send callback is required")
	}
	if chunkSize <= 0 {
		chunkSize = fileChunkSizeHub
	}

	buf := make([]byte, chunkSize)
	var offset int64
	sentDoneMarker := false

	for {
		n, readErr := body.Read(buf)
		if n > 0 {
			done := readErr == io.EOF
			payload := agentmgr.FileWriteData{
				RequestID: requestID,
				Path:      filePath,
				Data:      base64.StdEncoding.EncodeToString(buf[:n]),
				Offset:    offset,
				Done:      done,
			}
			if err := send(payload); err != nil {
				return offset, UploadRelaySendError{err: err}
			}
			offset += int64(n)
			if done {
				sentDoneMarker = true
			}
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return offset, readErr
		}
	}

	// Many readers return (0, io.EOF) after the final chunk, so emit an explicit
	// terminal marker whenever EOF was observed without a done=true chunk.
	if !sentDoneMarker {
		if err := send(agentmgr.FileWriteData{
			RequestID: requestID,
			Path:      filePath,
			Data:      "",
			Offset:    offset,
			Done:      true,
		}); err != nil {
			return offset, UploadRelaySendError{err: err}
		}
	}

	return offset, nil
}

type UploadRelaySendError struct {
	err error
}

func (e UploadRelaySendError) Error() string {
	return fmt.Sprintf("send upload chunk: %v", e.err)
}

func (e UploadRelaySendError) Unwrap() error {
	return e.err
}
