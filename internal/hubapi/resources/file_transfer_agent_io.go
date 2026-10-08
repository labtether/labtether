package resources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/fileproto"
	"io"
	"os"
	"time"
)

func (d *Deps) spoolAgentTransferSource(ctx context.Context, assetID, sourcePath string) (*os.File, int64, func(), error) {
	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		return nil, 0, nil, errors.New("source agent disconnected")
	}
	requestID := generateRequestID()
	bridge := newFileBridge(64, assetID)
	d.FileBridges.Store(requestID, bridge)
	cleanupBridge := func() {
		bridge.Close()
		d.FileBridges.Delete(requestID)
	}

	data, err := json.Marshal(agentmgr.FileReadData{RequestID: requestID, Path: sourcePath})
	if err != nil {
		cleanupBridge()
		return nil, 0, nil, err
	}
	if err := agentConn.Send(agentmgr.Message{Type: agentmgr.MsgFileRead, ID: requestID, Data: data}); err != nil {
		cleanupBridge()
		return nil, 0, nil, fmt.Errorf("send file read request: %w", err)
	}

	spool, err := os.CreateTemp("", "labtether-transfer-*")
	if err != nil {
		cleanupBridge()
		return nil, 0, nil, errors.New("create transfer spool")
	}
	cleanup := func() {
		cleanupBridge()
		_ = spool.Close()
		_ = os.Remove(spool.Name())
	}
	var written int64
	for {
		chunk, receiveErr := receiveAgentTransferChunk(ctx, bridge)
		if receiveErr != nil {
			cleanup()
			return nil, written, nil, receiveErr
		}
		if chunk.Error != "" {
			cleanup()
			return nil, written, nil, errors.New(chunk.Error)
		}
		payload, decodeErr := DecodeFileDownloadChunk(chunk)
		if decodeErr != nil {
			cleanup()
			return nil, written, nil, errFileDownloadInvalidAgentResponse
		}
		if writeErr := writeFileDownloadChunk(spool, payload, chunk.Offset, &written, fileproto.MaxTransferBytes); writeErr != nil {
			cleanup()
			return nil, written, nil, writeErr
		}
		if chunk.Done {
			break
		}
	}
	if err := spool.Sync(); err != nil {
		cleanup()
		return nil, written, nil, errors.New("sync transfer spool")
	}
	if _, err := spool.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, written, nil, errors.New("rewind transfer spool")
	}
	return spool, written, cleanup, nil
}

func receiveAgentTransferChunk(ctx context.Context, bridge *FileBridge) (agentmgr.FileDataPayload, error) {
	timer := time.NewTimer(fileRequestTimeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return agentmgr.FileDataPayload{}, ctx.Err()
	case msg := <-bridge.Ch:
		var chunk agentmgr.FileDataPayload
		if err := json.Unmarshal(msg.Data, &chunk); err != nil {
			return agentmgr.FileDataPayload{}, errFileDownloadInvalidAgentResponse
		}
		return chunk, nil
	case <-bridge.Done:
		if err := bridge.Err(); err != nil {
			return agentmgr.FileDataPayload{}, err
		}
		return agentmgr.FileDataPayload{}, errors.New("agent file stream closed")
	case <-timer.C:
		return agentmgr.FileDataPayload{}, errFileDownloadTimedOut
	}
}

func (d *Deps) writeAgentTransferDestination(
	ctx context.Context,
	assetID string,
	destPath string,
	source io.Reader,
	totalSize int64,
	progress fileproto.TransferProgress,
) (int64, error) {
	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		return 0, errors.New("destination agent disconnected")
	}
	requestID := generateRequestID()
	bridge := newFileBridge(1, assetID)
	d.FileBridges.Store(requestID, bridge)
	defer bridge.Close()
	defer d.FileBridges.Delete(requestID)

	progressReader := &fileTransferProgressReader{
		ctx:      ctx,
		reader:   newFileTransferBoundedReader(source),
		total:    totalSize,
		progress: progress,
	}
	sent, err := RelayFileUploadChunks(progressReader, requestID, destPath, fileChunkSizeHub, func(payload agentmgr.FileWriteData) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case msg := <-bridge.Ch:
			var result agentmgr.FileWrittenData
			if err := json.Unmarshal(msg.Data, &result); err != nil {
				return UploadAgentResponseError{err: err}
			}
			if result.Error != "" {
				return UploadAgentWriteError{message: result.Error}
			}
			return UploadAgentWriteError{message: "destination agent completed upload before source reached EOF"}
		default:
		}
		data, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return marshalErr
		}
		return agentConn.Send(agentmgr.Message{Type: agentmgr.MsgFileWrite, ID: requestID, Data: data})
	})
	if err != nil {
		return sent, err
	}
	if err := progressReader.terminalError(); err != nil {
		return sent, err
	}

	timer := time.NewTimer(fileRequestTimeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return sent, ctx.Err()
	case msg := <-bridge.Ch:
		var result agentmgr.FileWrittenData
		if err := json.Unmarshal(msg.Data, &result); err != nil {
			return sent, errFileDownloadInvalidAgentResponse
		}
		if result.Error != "" {
			return sent, errors.New(result.Error)
		}
		if result.BytesWritten != sent {
			return sent, fmt.Errorf("destination agent byte count mismatch: wrote %d, expected %d", result.BytesWritten, sent)
		}
		return sent, nil
	case <-bridge.Done:
		if err := bridge.Err(); err != nil {
			return sent, err
		}
		return sent, errors.New("destination agent response stream closed")
	case <-timer.C:
		return sent, errors.New("destination agent did not confirm file write in time")
	}
}

type fileTransferBoundedReader struct {
	reader      io.Reader
	remaining   int64
	terminalErr error
	finished    bool
}

func newFileTransferBoundedReader(reader io.Reader) *fileTransferBoundedReader {
	return &fileTransferBoundedReader{reader: reader, remaining: fileproto.MaxTransferBytes}
}

func (r *fileTransferBoundedReader) Read(payload []byte) (int, error) {
	if r.terminalErr != nil {
		return 0, r.terminalErr
	}
	if r.finished {
		return 0, io.EOF
	}
	if r.remaining == 0 {
		var probe [1]byte
		n, err := r.reader.Read(probe[:])
		if n > 0 {
			r.terminalErr = fileproto.ErrTransferTooLarge
			return 0, r.terminalErr
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				r.finished = true
			}
			return 0, err
		}
		return 0, nil
	}
	if int64(len(payload)) > r.remaining {
		payload = payload[:r.remaining]
	}
	n, err := r.reader.Read(payload)
	r.remaining -= int64(n)
	if err != nil {
		if errors.Is(err, io.EOF) {
			r.finished = true
		} else {
			r.terminalErr = err
		}
	}
	return n, err
}

type fileTransferProgressReader struct {
	ctx         context.Context
	reader      *fileTransferBoundedReader
	transferred int64
	total       int64
	progress    fileproto.TransferProgress
}

func (r *fileTransferProgressReader) Read(payload []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(payload)
	r.transferred += int64(n)
	if n > 0 && r.progress != nil {
		r.progress(r.transferred, r.total)
	}
	return n, err
}

func (r *fileTransferProgressReader) terminalError() error {
	return r.reader.terminalErr
}
