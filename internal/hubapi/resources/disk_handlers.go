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
		d.writeAgentNotConnected(w, assetID)
		return
	}

	d.handleDiskList(w, r, assetID)
}

func (d *Deps) handleDiskList(w http.ResponseWriter, r *http.Request, assetID string) {
	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		d.writeAgentNotConnected(w, assetID)
		return
	}

	requestID := generateRequestID()
	msg, err := d.requestAgent(d.DiskReplies, agentConn, agentmgr.MsgDiskList, requestID,
		agentmgr.DiskListData{RequestID: requestID}, assetID, diskRequestTimeout)
	if err != nil {
		d.writeAgentRequestFailure(w, assetID, err)
		return
	}
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
}

func (d *Deps) ProcessAgentDiskListed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.DiskListedData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid disk listed data: %v", err)
		return
	}
	deliverAgentReply(d.DiskReplies, matchPayloadID, agentmgr.MsgDiskListed, conn, msg, data.RequestID)
}
