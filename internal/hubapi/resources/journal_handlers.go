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

const journalQueryTimeout = 20 * time.Second

// journalBridge holds the channel for a pending historical journal query.

// handleJournalLogs dispatches /logs/journal/{assetId} requests.
func (d *Deps) HandleJournalLogs(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/logs/journal/")
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

	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent disconnected")
		return
	}

	requestID := generateRequestID()
	bridge := &JournalBridge{Ch: make(chan agentmgr.Message, 1), ExpectedAssetID: assetID}
	d.JournalBridges.Store(requestID, bridge)
	defer d.JournalBridges.Delete(requestID)

	reqPayload := agentmgr.JournalQueryData{
		RequestID: requestID,
		Since:     strings.TrimSpace(r.URL.Query().Get("since")),
		Until:     strings.TrimSpace(r.URL.Query().Get("until")),
		Unit:      strings.TrimSpace(r.URL.Query().Get("unit")),
		Priority:  strings.TrimSpace(r.URL.Query().Get("priority")),
		Search:    strings.TrimSpace(r.URL.Query().Get("q")),
		Limit:     parseLimit(r, 200),
	}

	data, _ := json.Marshal(reqPayload)
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgJournalQuery,
		ID:   requestID,
		Data: data,
	}); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to send request to agent")
		return
	}

	select {
	case msg := <-bridge.Ch:
		var listed agentmgr.JournalEntriesData
		if err := json.Unmarshal(msg.Data, &listed); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
			return
		}
		if listed.Error != "" {
			servicehttp.WriteError(w, http.StatusBadRequest, listed.Error)
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"entries": listed.Entries,
		})
	case <-time.After(journalQueryTimeout):
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
	}
}

func (d *Deps) ProcessAgentJournalEntries(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.JournalEntriesData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid journal entries data: %v", err)
		return
	}
	if raw, ok := d.JournalBridges.Load(data.RequestID); ok {
		bridge, ok := raw.(*JournalBridge)
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
