package resources

import (
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) HandleFileMkdir(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.enforceAssetActionGuard(w, assetID) {
		return
	}

	dirPath := r.URL.Query().Get("path")
	if dirPath == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "path is required")
		return
	}

	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		d.writeAgentNotConnected(w, assetID)
		return
	}
	if refuseUnadvertisedConn(w, agentConn, agentmgr.MsgFileMkdir) {
		return
	}

	requestID := generateRequestID()
	bridge := newFileBridge(1, assetID)
	d.FileBridges.Store(requestID, bridge)
	defer bridge.Close()
	defer d.FileBridges.Delete(requestID)

	msg, _ := agentmgr.NewMessage(agentmgr.MsgFileMkdir, requestID, agentmgr.FileMkdirData{
		RequestID: requestID,
		Path:      dirPath,
	})
	if err := agentConn.Send(msg); err != nil {
		d.writeAgentNotConnected(w, assetID)
		return
	}

	select {
	case <-r.Context().Done():
		return
	case <-bridge.Done:
		writeFileBridgeClosedError(w, bridge)
	case msg := <-bridge.Ch:
		result, err := agentmgr.Decode[agentmgr.FileResultData](msg)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
			return
		}
		if result.Error != "" {
			writeAgentFileError(w, missingParent, dirPath, result.Error)
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, result)
	case <-time.After(fileRequestTimeout):
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
	}
}

// handleFileDelete deletes a file or directory on the agent.
func (d *Deps) HandleFileDelete(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodDelete {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.enforceAssetActionGuard(w, assetID) {
		return
	}

	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "path is required")
		return
	}

	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		d.writeAgentNotConnected(w, assetID)
		return
	}
	if refuseUnadvertisedConn(w, agentConn, agentmgr.MsgFileDelete) {
		return
	}

	requestID := generateRequestID()
	bridge := newFileBridge(1, assetID)
	d.FileBridges.Store(requestID, bridge)
	defer bridge.Close()
	defer d.FileBridges.Delete(requestID)

	msg, _ := agentmgr.NewMessage(agentmgr.MsgFileDelete, requestID, agentmgr.FileDeleteData{
		RequestID: requestID,
		Path:      filePath,
	})
	if err := agentConn.Send(msg); err != nil {
		d.writeAgentNotConnected(w, assetID)
		return
	}

	select {
	case <-r.Context().Done():
		return
	case <-bridge.Done:
		writeFileBridgeClosedError(w, bridge)
	case msg := <-bridge.Ch:
		result, err := agentmgr.Decode[agentmgr.FileResultData](msg)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
			return
		}
		if result.Error != "" {
			writeAgentFileError(w, missingFile, filePath, result.Error)
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, result)
	case <-time.After(fileRequestTimeout):
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
	}
}

// handleFileRename renames/moves a file or directory on the agent.
func (d *Deps) HandleFileRename(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.enforceAssetActionGuard(w, assetID) {
		return
	}

	oldPath := r.URL.Query().Get("old_path")
	newPath := r.URL.Query().Get("new_path")
	if oldPath == "" || newPath == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "old_path and new_path are required")
		return
	}

	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		d.writeAgentNotConnected(w, assetID)
		return
	}
	if refuseUnadvertisedConn(w, agentConn, agentmgr.MsgFileRename) {
		return
	}

	requestID := generateRequestID()
	bridge := newFileBridge(1, assetID)
	d.FileBridges.Store(requestID, bridge)
	defer bridge.Close()
	defer d.FileBridges.Delete(requestID)

	msg, _ := agentmgr.NewMessage(agentmgr.MsgFileRename, requestID, agentmgr.FileRenameData{
		RequestID: requestID,
		OldPath:   oldPath,
		NewPath:   newPath,
	})
	if err := agentConn.Send(msg); err != nil {
		d.writeAgentNotConnected(w, assetID)
		return
	}

	select {
	case <-r.Context().Done():
		return
	case <-bridge.Done:
		writeFileBridgeClosedError(w, bridge)
	case msg := <-bridge.Ch:
		result, err := agentmgr.Decode[agentmgr.FileResultData](msg)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
			return
		}
		if result.Error != "" {
			writeAgentFileMoveError(w, oldPath, newPath, result.Error)
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, result)
	case <-time.After(fileRequestTimeout):
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
	}
}

// handleFileCopy copies a file or directory on the agent.
func (d *Deps) HandleFileCopy(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.enforceAssetActionGuard(w, assetID) {
		return
	}

	srcPath := r.URL.Query().Get("src_path")
	dstPath := r.URL.Query().Get("dst_path")
	if srcPath == "" || dstPath == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "src_path and dst_path are required")
		return
	}

	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		d.writeAgentNotConnected(w, assetID)
		return
	}
	if refuseUnadvertisedConn(w, agentConn, agentmgr.MsgFileCopy) {
		return
	}

	requestID := generateRequestID()
	bridge := newFileBridge(1, assetID)
	d.FileBridges.Store(requestID, bridge)
	defer bridge.Close()
	defer d.FileBridges.Delete(requestID)

	msg, _ := agentmgr.NewMessage(agentmgr.MsgFileCopy, requestID, agentmgr.FileCopyData{
		RequestID: requestID,
		SrcPath:   srcPath,
		DstPath:   dstPath,
	})
	if err := agentConn.Send(msg); err != nil {
		d.writeAgentNotConnected(w, assetID)
		return
	}

	select {
	case <-r.Context().Done():
		return
	case <-bridge.Done:
		writeFileBridgeClosedError(w, bridge)
	case msg := <-bridge.Ch:
		result, err := agentmgr.Decode[agentmgr.FileResultData](msg)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
			return
		}
		if result.Error != "" {
			writeAgentFileMoveError(w, srcPath, dstPath, result.Error)
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, result)
	case <-time.After(fileRequestTimeout):
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
	}
}
