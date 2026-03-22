package resources

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/servicehttp"
)

const diskRequestTimeout = 10 * time.Second

// diskBridge holds the channel for a pending disk list request.

// handleDisks dispatches /disks/{assetId} requests.
func (d *Deps) HandleDisks(w http.ResponseWriter, r *http.Request) {
	// Extract assetID from path: /disks/{assetId}
	path := strings.TrimPrefix(r.URL.Path, "/disks/")
	assetID := strings.TrimSpace(path)
	if assetID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset id required")
		return
	}

	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if d.AgentMgr == nil || !d.AgentMgr.IsConnected(assetID) {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent not connected")
		return
	}

	d.handleDiskList(w, r, assetID)
}

func (d *Deps) handleDiskList(w http.ResponseWriter, r *http.Request, assetID string) {
	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent disconnected")
		return
	}

	requestID := generateRequestID()

	bridge := &DiskBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: assetID,
	}
	d.DiskBridges.Store(requestID, bridge)
	defer d.DiskBridges.Delete(requestID)

	data, _ := json.Marshal(agentmgr.DiskListData{
		RequestID: requestID,
	})
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgDiskList,
		ID:   requestID,
		Data: data,
	}); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to send request to agent")
		return
	}

	select {
	case msg := <-bridge.Ch:
		var listed agentmgr.DiskListedData
		if err := json.Unmarshal(msg.Data, &listed); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
			return
		}
		if listed.Error != "" {
			servicehttp.WriteError(w, http.StatusBadRequest, listed.Error)
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, listed)
	case <-time.After(diskRequestTimeout):
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
	}
}

func (d *Deps) ProcessAgentDiskListed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.DiskListedData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid disk listed data: %v", err)
		return
	}
	if raw, ok := d.DiskBridges.Load(data.RequestID); ok {
		bridge, ok := raw.(*DiskBridge)
		if !ok || bridge == nil {
			return
		}
		if conn != nil && bridge.ExpectedAssetID != "" && !strings.EqualFold(strings.TrimSpace(conn.AssetID), strings.TrimSpace(bridge.ExpectedAssetID)) {
			return
		}
		select {
		case bridge.Ch <- msg:
		default:
		}
	}
}
