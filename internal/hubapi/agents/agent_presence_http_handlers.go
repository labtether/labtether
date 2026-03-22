package agents

import (
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

// handleConnectedAgents returns the list of connected agent asset IDs.
func (d *Deps) HandleConnectedAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	assets := d.AgentMgr.ConnectedAssets()
	assetsInfo := d.AgentMgr.ConnectedAssetsInfo()
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"count":      d.AgentMgr.Count(),
		"assets":     assets,
		"assetsInfo": assetsInfo,
	})
}

// handleAgentPresence returns detailed presence records for all connected agents.
func (d *Deps) HandleAgentPresence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if d.PresenceStore == nil {
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"count":    0,
			"presence": []any{},
		})
		return
	}
	records, err := d.PresenceStore.ListPresence()
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list presence")
		return
	}
	if records == nil {
		records = []persistence.AgentPresence{}
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"count":    len(records),
		"presence": records,
	})
}

// sendShutdownToAgents sends a CloseGoingAway frame to all connected agents
// before the hub shuts down, so agents know to reconnect immediately.
func (d *Deps) SendShutdownToAgents() {
	assets := d.AgentMgr.ConnectedAssets()
	for _, assetID := range assets {
		if conn, ok := d.AgentMgr.Get(assetID); ok {
			msg := websocket.FormatCloseMessage(websocket.CloseGoingAway, "hub shutting down")
			_ = conn.WriteClose(msg)
		}
	}
}
