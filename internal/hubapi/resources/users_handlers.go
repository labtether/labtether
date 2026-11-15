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

const usersRequestTimeout = 10 * time.Second

// handleUsers dispatches /users/{assetId} requests.
func (d *Deps) HandleUsers(w http.ResponseWriter, r *http.Request) {
	// Extract assetID from path: /users/{assetId}
	path := strings.TrimPrefix(r.URL.Path, "/users/")
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

	d.handleUsersList(w, r, assetID)
}

func (d *Deps) handleUsersList(w http.ResponseWriter, r *http.Request, assetID string) {
	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		d.writeAgentNotConnected(w, assetID)
		return
	}

	requestID := generateRequestID()
	msg, err := d.requestAgent(d.UsersReplies, agentConn, agentmgr.MsgUsersList, requestID,
		agentmgr.UsersListData{RequestID: requestID}, assetID, usersRequestTimeout)
	if err != nil {
		d.writeAgentRequestFailure(w, assetID, err)
		return
	}
	var listed agentmgr.UsersListedData
	if err := json.Unmarshal(msg.Data, &listed); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
		return
	}
	if listed.Error != "" {
		if !writeMissingCommand(w, listed.Error, "signed-in users can't be listed") {
			servicehttp.WriteError(w, http.StatusBadRequest, listed.Error)
		}
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, listed)
}

func (d *Deps) ProcessAgentUsersListed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.UsersListedData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid users listed data: %v", err)
		return
	}
	deliverAgentReply(d.UsersReplies, matchListAcceptsNoConn, conn, msg, data.RequestID)
}
