package resources

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) HandleFileMkdir(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	dirPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if dirPath == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "path is required")
		return
	}

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

	data, _ := json.Marshal(agentmgr.FileMkdirData{
		RequestID: requestID,
		Path:      dirPath,
	})
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgFileMkdir,
		ID:   requestID,
		Data: data,
	}); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to send request to agent")
		return
	}

	select {
	case msg := <-bridge.Ch:
		var result agentmgr.FileResultData
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

// handleFileDelete deletes a file or directory on the agent.
func (d *Deps) HandleFileDelete(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodDelete {
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
	bridge := newFileBridge(1, assetID)
	d.FileBridges.Store(requestID, bridge)
	defer bridge.Close()
	defer d.FileBridges.Delete(requestID)

	data, _ := json.Marshal(agentmgr.FileDeleteData{
		RequestID: requestID,
		Path:      filePath,
	})
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgFileDelete,
		ID:   requestID,
		Data: data,
	}); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to send request to agent")
		return
	}

	select {
	case msg := <-bridge.Ch:
		var result agentmgr.FileResultData
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

// handleFileRename renames/moves a file or directory on the agent.
func (d *Deps) HandleFileRename(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	oldPath := strings.TrimSpace(r.URL.Query().Get("old_path"))
	newPath := strings.TrimSpace(r.URL.Query().Get("new_path"))
	if oldPath == "" || newPath == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "old_path and new_path are required")
		return
	}

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

	data, _ := json.Marshal(agentmgr.FileRenameData{
		RequestID: requestID,
		OldPath:   oldPath,
		NewPath:   newPath,
	})
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgFileRename,
		ID:   requestID,
		Data: data,
	}); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to send request to agent")
		return
	}

	select {
	case msg := <-bridge.Ch:
		var result agentmgr.FileResultData
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

// handleFileCopy copies a file or directory on the agent.
func (d *Deps) HandleFileCopy(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	srcPath := strings.TrimSpace(r.URL.Query().Get("src_path"))
	dstPath := strings.TrimSpace(r.URL.Query().Get("dst_path"))
	if srcPath == "" || dstPath == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "src_path and dst_path are required")
		return
	}

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

	data, _ := json.Marshal(agentmgr.FileCopyData{
		RequestID: requestID,
		SrcPath:   srcPath,
		DstPath:   dstPath,
	})
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgFileCopy,
		ID:   requestID,
		Data: data,
	}); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to send request to agent")
		return
	}

	select {
	case msg := <-bridge.Ch:
		var result agentmgr.FileResultData
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
