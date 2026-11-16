package desktop

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

const clipboardRequestTimeout = 10 * time.Second

// requestClipboard sends one clipboard message and waits for the reply of
// replyType from the connection's own device, until the HTTP request ends or
// the connection closes.
func (d *Deps) requestClipboard(r *http.Request, agentConn *agentmgr.AgentConn, msgType, replyType, requestID string, payload any) (agentmgr.Message, error) {
	return agentmgr.Request(r.Context(), d.ClipboardReplies, agentmgr.Call[any, agentmgr.Message]{
		Type:        msgType,
		ID:          requestID,
		Payload:     payload,
		Send:        agentConn.Send,
		Device:      strings.TrimSpace(agentConn.AssetID),
		Accept:      func(msg agentmgr.Message) bool { return msg.Type == replyType },
		TimeoutFunc: d.replyWaitFunc(clipboardRequestTimeout),
		Disconnect:  agentConn.Done(),
	})
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
		d.writeAgentNotConnected(w, assetID)
		return
	}
	msgType := map[string]string{"get": agentmgr.MsgClipboardGet, "set": agentmgr.MsgClipboardSet}[action]
	if conn, ok := d.AgentMgr.Get(assetID); ok && msgType != "" && refuseUnadvertised(w, conn, msgType) {
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
		d.writeAgentNotConnected(w, assetID)
		return
	}

	// Parse optional format from request body.
	format := "text"
	if r.Body != nil {
		var body struct {
			Format string `json:"format"`
		}
		if _, err := shared.DecodeOptionalJSONBody(w, r, &body); err != nil {
			servicehttp.WriteError(w, shared.JSONDecodeErrorStatus(err), "invalid clipboard request payload")
			return
		} else if body.Format != "" {
			format = body.Format
		}
	}

	requestID := d.GenerateRequestID()

	msg, err := d.requestClipboard(r, agentConn, agentmgr.MsgClipboardGet, agentmgr.MsgClipboardData, requestID,
		agentmgr.ClipboardGetData{RequestID: requestID, Format: format})
	if err != nil {
		// A request whose caller went away writes nothing.
		if agentmgr.RequestKind(err) != agentmgr.RequestCanceled {
			d.writeAgentRequestFailure(w, assetID, err)
		}
		return
	}
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
}

// handleClipboardSet writes content to the agent's clipboard.
func (d *Deps) handleClipboardSet(w http.ResponseWriter, r *http.Request, assetID string) {
	if !d.enforceAssetActionGuard(w, assetID) {
		return
	}
	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		d.writeAgentNotConnected(w, assetID)
		return
	}

	var body agentmgr.ClipboardSetData
	if err := shared.DecodeJSONBody(w, r, &body); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	requestID := d.GenerateRequestID()
	body.RequestID = requestID

	msg, err := d.requestClipboard(r, agentConn, agentmgr.MsgClipboardSet, agentmgr.MsgClipboardSetAck, requestID, body)
	if err != nil {
		if agentmgr.RequestKind(err) != agentmgr.RequestCanceled {
			d.writeAgentRequestFailure(w, assetID, err)
		}
		return
	}
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
}

// ProcessAgentClipboardData handles clipboard.data from agent.
func (d *Deps) ProcessAgentClipboardData(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.ClipboardDataPayload
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}
	d.deliverClipboardResponse(agentmgr.MsgClipboardData, data.RequestID, conn, msg)
}

// ProcessAgentClipboardSetAck handles clipboard.set_ack from agent.
func (d *Deps) ProcessAgentClipboardSetAck(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.ClipboardSetAckData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}
	d.deliverClipboardResponse(agentmgr.MsgClipboardSetAck, data.RequestID, conn, msg)
}

// deliverClipboardResponse routes an agent clipboard response of replyType to
// the waiting HTTP handler. The router calls each handler for its own type.
func (d *Deps) deliverClipboardResponse(replyType, requestID string, conn *agentmgr.AgentConn, msg agentmgr.Message) {
	msg.Type = replyType
	d.ClipboardReplies.Deliver(matchClipboard, agentmgr.Reply[agentmgr.Message]{
		EnvelopeID: msg.ID, PayloadID: requestID, Conn: conn, Value: msg,
	})
}
