package desktop

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/servicehttp"
)

const displayRequestTimeout = 10 * time.Second

// HandleNodeSubRoutes dispatches /api/v1/nodes/{id}/{resource}[/...] requests
// to the appropriate sub-handler based on the resource segment.
func (d *Deps) HandleNodeSubRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/nodes/")
	if path == "" || path == r.URL.Path {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	parts := strings.SplitN(path, "/", 3) // [id, resource, ...]
	if len(parts) < 2 {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	assetID := strings.TrimSpace(parts[0])
	resource := strings.TrimSpace(parts[1])
	if assetID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset id required")
		return
	}

	switch resource {
	case "clipboard":
		// Delegate to existing clipboard handler (which expects full path parsing).
		d.HandleClipboardRoutes(w, r)
	case "displays":
		d.handleDisplayList(w, r, assetID)
	default:
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
	}
}

// DisplayBridge holds the channel for a pending display list request.
type DisplayBridge struct {
	Ch              chan agentmgr.DisplayListData
	ExpectedAssetID string
}

// HandleDisplayListDirect queries the list of displays/monitors on a remote agent.
// This is the exported variant for callers that already resolved the asset ID.
// Endpoint: GET /api/v1/nodes/{id}/displays
func (d *Deps) HandleDisplayListDirect(w http.ResponseWriter, r *http.Request, assetID string) {
	d.handleDisplayList(w, r, assetID)
}

// handleDisplayList queries the list of displays/monitors on a remote agent.
// Endpoint: GET /api/v1/nodes/{id}/displays
func (d *Deps) handleDisplayList(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.AgentMgr == nil || !d.AgentMgr.IsConnected(assetID) {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "agent not connected")
		return
	}

	requestID := idgen.New("dreq")
	bridge := &DisplayBridge{
		Ch:              make(chan agentmgr.DisplayListData, 1),
		ExpectedAssetID: assetID,
	}
	d.DisplayBridges.Store(requestID, bridge)
	defer d.DisplayBridges.Delete(requestID)

	payload, _ := json.Marshal(map[string]string{"request_id": requestID})
	if err := d.AgentMgr.SendToAgent(assetID, agentmgr.Message{
		Type: agentmgr.MsgDesktopListDisplays,
		ID:   requestID,
		Data: payload,
	}); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to request displays from agent")
		return
	}

	select {
	case result := <-bridge.Ch:
		if strings.TrimSpace(result.Error) != "" {
			servicehttp.WriteError(w, http.StatusBadGateway, result.Error)
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"displays": result.Displays,
		})
	case <-time.After(displayRequestTimeout):
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
	}
}

// ProcessAgentDesktopDisplays handles desktop.displays from agent.
func (d *Deps) ProcessAgentDesktopDisplays(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.DisplayListData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("display: invalid desktop.displays payload: %v", err)
		return
	}
	if raw, ok := d.DisplayBridges.Load(data.RequestID); ok {
		bridge, ok := raw.(*DisplayBridge)
		if !ok || bridge == nil {
			return
		}
		if conn == nil || (strings.TrimSpace(bridge.ExpectedAssetID) != "" && !strings.EqualFold(strings.TrimSpace(bridge.ExpectedAssetID), strings.TrimSpace(conn.AssetID))) {
			return
		}
		select {
		case bridge.Ch <- data:
		default:
		}
	}
}
