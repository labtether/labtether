package resources

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/servicehttp"
)

const cronRequestTimeout = 10 * time.Second

// handleCrons dispatches /cron/{assetId} requests.
func (d *Deps) HandleCrons(w http.ResponseWriter, r *http.Request) {
	// Extract assetID from path: /cron/{assetId}
	path := strings.TrimPrefix(r.URL.Path, "/cron/")
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

	d.handleCronList(w, r, assetID)
}

func (d *Deps) handleCronList(w http.ResponseWriter, r *http.Request, assetID string) {
	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		d.writeAgentNotConnected(w, assetID)
		return
	}

	requestID := generateRequestID()
	msg, err := d.requestAgent(d.CronReplies, agentConn, agentmgr.MsgCronList, requestID,
		agentmgr.CronListData{RequestID: requestID}, assetID, cronRequestTimeout)
	if err != nil {
		d.writeAgentRequestFailure(w, assetID, err)
		return
	}
	listed, err := agentmgr.Decode[agentmgr.CronListedData](msg)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
		return
	}
	if listed.Error != "" {
		if !writeMissingCommand(w, listed.Error, "cron jobs can't be listed") {
			servicehttp.WriteError(w, http.StatusBadRequest, listed.Error)
		}
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, listed)
}

func (d *Deps) ProcessAgentCronListed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	data, err := agentmgr.Decode[agentmgr.CronListedData](msg)
	if err != nil {
		log.Printf("agentws: invalid cron listed data: %v", err)
		return
	}
	deliverAgentReply(d.CronReplies, matchPayloadID, agentmgr.MsgCronListed, conn, msg, data.RequestID)
}
