package desktop

import (
	"context"
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

// HandleDisplayListDirect queries the list of displays/monitors on a remote agent.
// This is the exported variant for callers that already resolved the asset ID.
// Endpoint: GET /api/v1/nodes/{id}/displays
func (d *Deps) HandleDisplayListDirect(w http.ResponseWriter, r *http.Request, assetID string) {
	d.handleDisplayList(w, r, assetID)
}

// handleDisplayList queries the list of displays/monitors on a remote agent,
// and says whether this Hub's RDP gateway is running.
// Endpoint: GET /api/v1/nodes/{id}/displays
func (d *Deps) handleDisplayList(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var agentConn *agentmgr.AgentConn
	if d.AgentMgr != nil {
		agentConn, _ = d.AgentMgr.Get(assetID)
	}
	if agentConn == nil {
		d.writeAgentNotConnected(w, assetID)
		return
	}
	if refuseUnadvertised(w, agentConn, agentmgr.MsgDesktopListDisplays) {
		return
	}

	// Asked while the agent answers, so a Hub without the gateway adds no wait.
	rdpGateway := make(chan bool, 1)
	go func() { rdpGateway <- rdpGatewayReachable(r.Context()) }()

	requestID := idgen.New("dreq")
	result, err := agentmgr.Request(context.Background(), d.DisplayReplies, agentmgr.Call[map[string]string, agentmgr.DisplayListData]{
		Type:    agentmgr.MsgDesktopListDisplays,
		ID:      requestID,
		Payload: map[string]string{"request_id": requestID},
		// Sent on the connection looked up above, so the wait can watch
		// the connection the request went out on.
		Send:        agentConn.Send,
		Device:      assetID,
		TimeoutFunc: d.replyWaitFunc(displayRequestTimeout),
		Disconnect:  agentConn.Done(),
	})
	if err != nil {
		d.writeAgentRequestFailure(w, assetID, err)
		return
	}
	if strings.TrimSpace(result.Error) != "" {
		servicehttp.WriteError(w, http.StatusBadGateway, result.Error)
		return
	}
	// rdp_gateway tells the console whether this Hub can serve RDP, so it
	// does not offer or recommend a protocol that cannot start.
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"displays":    result.Displays,
		"rdp_gateway": <-rdpGateway,
	})
}

// ProcessAgentDesktopDisplays handles desktop.displays from agent.
func (d *Deps) ProcessAgentDesktopDisplays(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.DisplayListData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("display: invalid desktop.displays payload: %v", err)
		return
	}
	d.DisplayReplies.Deliver(matchDisplays, agentmgr.Reply[agentmgr.DisplayListData]{
		EnvelopeID: msg.ID, PayloadID: data.RequestID, Conn: conn, Value: data,
	})
}
