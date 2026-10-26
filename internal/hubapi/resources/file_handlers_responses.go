package resources

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
)

var errFileResponseBackpressured = errors.New("file response bridge backpressure exceeded")
var errFileAgentDisconnected = errors.New("agent disconnected")

func writeFileBridgeClosedError(w http.ResponseWriter, bridge *FileBridge) {
	message := "agent response stream closed"
	if errors.Is(bridge.Err(), errFileAgentDisconnected) {
		message = errFileAgentDisconnected.Error()
	}
	servicehttp.WriteError(w, http.StatusBadGateway, message)
}

// FailAgentFileOperations runs inside the agent's disconnect transition,
// before a replacement connection can accept new requests for this asset.
func (d *Deps) FailAgentFileOperations(assetID string) {
	if d.FileBridges == nil {
		return
	}
	d.FileBridges.Range(func(key, raw any) bool {
		bridge, ok := raw.(*FileBridge)
		if ok && bridge != nil && bridge.ExpectedAssetID == assetID {
			bridge.Fail(errFileAgentDisconnected)
			d.FileBridges.CompareAndDelete(key, bridge)
		}
		return true
	})
}

func (d *Deps) ProcessAgentFileListed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.FileListedData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}
	d.deliverFileResponseFromAgent(conn, data.RequestID, msg)
}

// processAgentFileData handles file.data from agent.
func (d *Deps) ProcessAgentFileData(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.FileDataPayload
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}
	d.deliverFileResponseFromAgent(conn, data.RequestID, msg)
}

// processAgentFileWritten handles file.written from agent.
func (d *Deps) ProcessAgentFileWritten(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.FileWrittenData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}
	d.deliverFileResponseFromAgent(conn, data.RequestID, msg)
}

// processAgentFileResult handles file.result from agent.
func (d *Deps) ProcessAgentFileResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.FileResultData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}
	d.deliverFileResponseFromAgent(conn, data.RequestID, msg)
}

func (d *Deps) deliverFileResponseFromAgent(conn *agentmgr.AgentConn, requestID string, msg agentmgr.Message) {
	if raw, ok := d.FileBridges.Load(requestID); ok {
		bridge, ok := raw.(*FileBridge)
		if !ok || bridge == nil {
			return
		}
		expectedAssetID := strings.TrimSpace(bridge.ExpectedAssetID)
		if expectedAssetID != "" {
			if conn == nil || !strings.EqualFold(expectedAssetID, strings.TrimSpace(conn.AssetID)) {
				return
			}
		}
		d.DeliverFileResponse(requestID, msg)
	}
}

// deliverFileResponse routes an agent file response to the waiting HTTP handler.
func (d *Deps) DeliverFileResponse(requestID string, msg agentmgr.Message) {
	if raw, ok := d.FileBridges.Load(requestID); ok {
		bridge, ok := raw.(*FileBridge)
		if !ok || bridge == nil {
			return
		}
		timer := time.NewTimer(fileResponseBackpressureLimit)
		defer timer.Stop()
		select {
		case bridge.Ch <- msg:
		case <-bridge.Done:
		case <-timer.C:
			securityruntime.Logf("file: dropping stalled response stream for request %s after %s of backpressure", requestID, fileResponseBackpressureLimit)
			bridge.Fail(errFileResponseBackpressured)
		}
	}
}

// generateRequestID creates a unique request ID for file operations.
func generateRequestID() string { return shared.GenerateRequestID() }
