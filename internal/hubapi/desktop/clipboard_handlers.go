package desktop

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/servicehttp"
)

const clipboardRequestTimeout = 10 * time.Second

// ClipboardBridge holds the channel for a pending clipboard request.
type ClipboardBridge struct {
	Ch              chan agentmgr.Message
	ExpectedAgentID string
}

// HandleClipboardRoutes dispatches /api/v1/nodes/{id}/clipboard/{action} requests.
func (d *Deps) HandleClipboardRoutes(w http.ResponseWriter, r *http.Request) {
	// Path: /api/v1/nodes/{id}/clipboard/{action}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/nodes/")
	if path == "" || path == r.URL.Path {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	parts := strings.SplitN(path, "/", 3) // [id, "clipboard", action]
	if len(parts) < 3 || parts[1] != "clipboard" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	assetID := strings.TrimSpace(parts[0])
	action := strings.TrimSpace(parts[2])
	if assetID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset id required")
		return
	}

	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if d.AgentMgr == nil || !d.AgentMgr.IsConnected(assetID) {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent not connected")
		return
	}

	switch action {
	case "get":
		d.handleClipboardGet(w, r, assetID)
	case "set":
		d.handleClipboardSet(w, r, assetID)
	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown clipboard action")
	}
}

// handleClipboardGet requests clipboard contents from the agent.
func (d *Deps) handleClipboardGet(w http.ResponseWriter, r *http.Request, assetID string) {
	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent disconnected")
		return
	}

	// Parse optional format from request body.
	format := "text"
	if r.Body != nil {
		var body struct {
			Format string `json:"format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.Format != "" {
			format = body.Format
		}
	}

	requestID := d.GenerateRequestID()

	bridge := &ClipboardBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAgentID: strings.TrimSpace(agentConn.AssetID),
	}
	d.ClipboardBridges.Store(requestID, bridge)
	defer d.ClipboardBridges.Delete(requestID)

	data, _ := json.Marshal(agentmgr.ClipboardGetData{
		RequestID: requestID,
		Format:    format,
	})
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgClipboardGet,
		ID:   requestID,
		Data: data,
	}); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to send request to agent")
		return
	}

	select {
	case msg := <-bridge.Ch:
		var payload agentmgr.ClipboardDataPayload
		if err := json.Unmarshal(msg.Data, &payload); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
			return
		}
		if payload.Error != "" {
			servicehttp.WriteError(w, http.StatusBadRequest, payload.Error)
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, payload)
	case <-time.After(clipboardRequestTimeout):
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
	}
}

// handleClipboardSet writes content to the agent's clipboard.
func (d *Deps) handleClipboardSet(w http.ResponseWriter, r *http.Request, assetID string) {
	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent disconnected")
		return
	}

	var body agentmgr.ClipboardSetData
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	requestID := d.GenerateRequestID()
	body.RequestID = requestID

	bridge := &ClipboardBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAgentID: strings.TrimSpace(agentConn.AssetID),
	}
	d.ClipboardBridges.Store(requestID, bridge)
	defer d.ClipboardBridges.Delete(requestID)

	data, _ := json.Marshal(body)
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgClipboardSet,
		ID:   requestID,
		Data: data,
	}); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to send request to agent")
		return
	}

	select {
	case msg := <-bridge.Ch:
		var ack agentmgr.ClipboardSetAckData
		if err := json.Unmarshal(msg.Data, &ack); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
			return
		}
		if ack.Error != "" {
			servicehttp.WriteError(w, http.StatusBadRequest, ack.Error)
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, ack)
	case <-time.After(clipboardRequestTimeout):
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
	}
}

// ProcessAgentClipboardData handles clipboard.data from agent.
func (d *Deps) ProcessAgentClipboardData(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.ClipboardDataPayload
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}
	d.deliverClipboardResponse(data.RequestID, conn, msg)
}

// ProcessAgentClipboardSetAck handles clipboard.set_ack from agent.
func (d *Deps) ProcessAgentClipboardSetAck(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.ClipboardSetAckData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}
	d.deliverClipboardResponse(data.RequestID, conn, msg)
}

// deliverClipboardResponse routes an agent clipboard response to the waiting HTTP handler.
func (d *Deps) deliverClipboardResponse(requestID string, conn *agentmgr.AgentConn, msg agentmgr.Message) {
	if raw, ok := d.ClipboardBridges.Load(requestID); ok {
		bridge, ok := raw.(*ClipboardBridge)
		if !ok || bridge == nil {
			return
		}
		if bridge.ExpectedAgentID != "" {
			if conn == nil || strings.TrimSpace(conn.AssetID) != bridge.ExpectedAgentID {
				return
			}
		}
		select {
		case bridge.Ch <- msg:
		default:
		}
	}
}
