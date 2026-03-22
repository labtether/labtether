package resources

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/servicehttp"
)

const (
	networkRequestTimeout = 10 * time.Second
	networkActionTimeout  = 90 * time.Second
)

var validNetworkActions = map[string]bool{
	"apply":    true,
	"rollback": true,
}

// networkBridge holds the channel for a pending network request.

// handleNetworks dispatches /network/{assetId} and /network/{assetId}/{action} requests.
func (d *Deps) HandleNetworks(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/network/")
	if path == "" || path == r.URL.Path {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset id required")
		return
	}

	parts := strings.SplitN(path, "/", 2)
	assetID := strings.TrimSpace(parts[0])
	if assetID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset id required")
		return
	}

	action := ""
	if len(parts) > 1 {
		action = strings.TrimSpace(parts[1])
	}

	if d.AgentMgr == nil || !d.AgentMgr.IsConnected(assetID) {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent not connected")
		return
	}

	if action == "" {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		d.handleNetworkList(w, r, assetID)
		return
	}

	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	d.handleNetworkAction(w, r, assetID, action)
}

func (d *Deps) handleNetworkList(w http.ResponseWriter, r *http.Request, assetID string) {
	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent disconnected")
		return
	}

	requestID := generateRequestID()
	bridge := &NetworkBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: assetID,
	}
	d.NetworkBridges.Store(requestID, bridge)
	defer d.NetworkBridges.Delete(requestID)

	data, _ := json.Marshal(agentmgr.NetworkListData{RequestID: requestID})
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgNetworkList,
		ID:   requestID,
		Data: data,
	}); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to send request to agent")
		return
	}

	select {
	case msg := <-bridge.Ch:
		var listed agentmgr.NetworkListedData
		if err := json.Unmarshal(msg.Data, &listed); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
			return
		}
		if listed.Error != "" {
			servicehttp.WriteError(w, http.StatusBadRequest, listed.Error)
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, listed)
	case <-time.After(networkRequestTimeout):
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
	}
}

func (d *Deps) handleNetworkAction(w http.ResponseWriter, r *http.Request, assetID, action string) {
	if !validNetworkActions[action] {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid action: must be apply or rollback")
		return
	}

	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent disconnected")
		return
	}

	var body struct {
		Method       string `json:"method"`
		Connection   string `json:"connection"`
		VerifyTarget string `json:"verify_target"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil && !errors.Is(err, io.EOF) {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	requestID := generateRequestID()
	bridge := &NetworkBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: assetID,
	}
	d.NetworkBridges.Store(requestID, bridge)
	defer d.NetworkBridges.Delete(requestID)

	data, _ := json.Marshal(agentmgr.NetworkActionData{
		RequestID:    requestID,
		Action:       action,
		Method:       strings.TrimSpace(body.Method),
		Connection:   strings.TrimSpace(body.Connection),
		VerifyTarget: strings.TrimSpace(body.VerifyTarget),
	})
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgNetworkAction,
		ID:   requestID,
		Data: data,
	}); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to send request to agent")
		return
	}

	select {
	case msg := <-bridge.Ch:
		var result agentmgr.NetworkResultData
		if err := json.Unmarshal(msg.Data, &result); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
			return
		}
		if result.Error != "" {
			servicehttp.WriteJSON(w, http.StatusBadRequest, result)
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, result)
	case <-time.After(networkActionTimeout):
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
	}
}

func (d *Deps) ProcessAgentNetworkListed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.NetworkListedData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid network listed data: %v", err)
		return
	}
	if raw, ok := d.NetworkBridges.Load(data.RequestID); ok {
		bridge, ok := raw.(*NetworkBridge)
		if !ok || bridge == nil {
			return
		}
		if conn == nil || (strings.TrimSpace(bridge.ExpectedAssetID) != "" && !strings.EqualFold(strings.TrimSpace(bridge.ExpectedAssetID), strings.TrimSpace(conn.AssetID))) {
			return
		}
		select {
		case bridge.Ch <- msg:
		default:
		}
	}
}

func (d *Deps) ProcessAgentNetworkResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.NetworkResultData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid network result data: %v", err)
		return
	}
	if raw, ok := d.NetworkBridges.Load(data.RequestID); ok {
		bridge, ok := raw.(*NetworkBridge)
		if !ok || bridge == nil {
			return
		}
		if conn == nil || (strings.TrimSpace(bridge.ExpectedAssetID) != "" && !strings.EqualFold(strings.TrimSpace(bridge.ExpectedAssetID), strings.TrimSpace(conn.AssetID))) {
			return
		}
		select {
		case bridge.Ch <- msg:
		default:
		}
	}
}
