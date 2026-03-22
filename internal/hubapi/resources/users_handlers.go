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

// usersBridge holds the channel for a pending users list request.

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
		servicehttp.WriteError(w, http.StatusBadGateway, "agent not connected")
		return
	}

	d.handleUsersList(w, r, assetID)
}

func (d *Deps) handleUsersList(w http.ResponseWriter, r *http.Request, assetID string) {
	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent disconnected")
		return
	}

	requestID := generateRequestID()

	bridge := &UsersBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: assetID,
	}
	d.UsersBridges.Store(requestID, bridge)
	defer d.UsersBridges.Delete(requestID)

	data, _ := json.Marshal(agentmgr.UsersListData{
		RequestID: requestID,
	})
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgUsersList,
		ID:   requestID,
		Data: data,
	}); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to send request to agent")
		return
	}

	select {
	case msg := <-bridge.Ch:
		var listed agentmgr.UsersListedData
		if err := json.Unmarshal(msg.Data, &listed); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
			return
		}
		if listed.Error != "" {
			servicehttp.WriteError(w, http.StatusBadRequest, listed.Error)
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, listed)
	case <-time.After(usersRequestTimeout):
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
	}
}

func (d *Deps) ProcessAgentUsersListed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.UsersListedData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid users listed data: %v", err)
		return
	}
	if raw, ok := d.UsersBridges.Load(data.RequestID); ok {
		bridge, ok := raw.(*UsersBridge)
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
