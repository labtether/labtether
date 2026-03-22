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

const (
	serviceListTimeout   = 10 * time.Second
	serviceActionTimeout = 30 * time.Second
)

var validServiceActions = map[string]bool{
	"start":   true,
	"stop":    true,
	"restart": true,
	"enable":  true,
	"disable": true,
}

// serviceBridge holds the channel for a pending service request.

// handleServices dispatches /services/{assetId} and /services/{assetId}/{action} requests.
func (d *Deps) HandleServices(w http.ResponseWriter, r *http.Request) {
	// Extract path after /services/
	path := strings.TrimPrefix(r.URL.Path, "/services/")
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
		d.handleServiceList(w, r, assetID)
		return
	}

	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	d.handleServiceAction(w, r, assetID, action)
}

func (d *Deps) handleServiceList(w http.ResponseWriter, r *http.Request, assetID string) {
	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent disconnected")
		return
	}

	requestID := generateRequestID()

	bridge := &ServiceBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: assetID,
	}
	d.ServiceBridges.Store(requestID, bridge)
	defer d.ServiceBridges.Delete(requestID)

	data, _ := json.Marshal(agentmgr.ServiceListData{
		RequestID: requestID,
	})
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgServiceList,
		ID:   requestID,
		Data: data,
	}); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to send request to agent")
		return
	}

	select {
	case msg := <-bridge.Ch:
		var listed agentmgr.ServiceListedData
		if err := json.Unmarshal(msg.Data, &listed); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
			return
		}
		if listed.Error != "" {
			servicehttp.WriteError(w, http.StatusBadRequest, listed.Error)
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, listed)
	case <-time.After(serviceListTimeout):
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
	}
}

func (d *Deps) handleServiceAction(w http.ResponseWriter, r *http.Request, assetID, action string) {
	if !validServiceActions[action] {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid action: must be one of start, stop, restart, enable, disable")
		return
	}

	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent disconnected")
		return
	}

	var body struct {
		Service string `json:"service"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(body.Service) == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "service name required")
		return
	}

	requestID := generateRequestID()

	bridge := &ServiceBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: assetID,
	}
	d.ServiceBridges.Store(requestID, bridge)
	defer d.ServiceBridges.Delete(requestID)

	data, _ := json.Marshal(agentmgr.ServiceActionData{
		RequestID: requestID,
		Service:   body.Service,
		Action:    action,
	})
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgServiceAction,
		ID:   requestID,
		Data: data,
	}); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to send request to agent")
		return
	}

	select {
	case msg := <-bridge.Ch:
		var result agentmgr.ServiceResultData
		if err := json.Unmarshal(msg.Data, &result); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
			return
		}
		if result.Error != "" {
			servicehttp.WriteError(w, http.StatusBadRequest, result.Error)
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, result)
	case <-time.After(serviceActionTimeout):
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
	}
}

func (d *Deps) ProcessAgentServiceListed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.ServiceListedData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid service listed data: %v", err)
		return
	}
	if raw, ok := d.ServiceBridges.Load(data.RequestID); ok {
		bridge, ok := raw.(*ServiceBridge)
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

func (d *Deps) ProcessAgentServiceResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.ServiceResultData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid service result data: %v", err)
		return
	}
	if raw, ok := d.ServiceBridges.Load(data.RequestID); ok {
		bridge, ok := raw.(*ServiceBridge)
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
