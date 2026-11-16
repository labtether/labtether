package resources

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/servicehttp"
)

const journalQueryTimeout = 20 * time.Second

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
		d.writeAgentNotConnected(w, assetID)
		return
	}

	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		d.writeAgentNotConnected(w, assetID)
		return
	}

	requestID := generateRequestID()
	reqPayload := agentmgr.JournalQueryData{
		RequestID: requestID,
		Since:     strings.TrimSpace(r.URL.Query().Get("since")),
		Until:     strings.TrimSpace(r.URL.Query().Get("until")),
		Unit:      strings.TrimSpace(r.URL.Query().Get("unit")),
		Priority:  strings.TrimSpace(r.URL.Query().Get("priority")),
		Search:    strings.TrimSpace(r.URL.Query().Get("q")),
		Limit:     parseLimit(r, 200),
	}

	msg, err := d.requestAgent(d.JournalReplies, agentConn, agentmgr.MsgJournalQuery, requestID,
		reqPayload, assetID, journalQueryTimeout)
	if err != nil {
		d.writeAgentRequestFailure(w, assetID, err)
		return
	}
	listed, err := agentmgr.Decode[agentmgr.JournalEntriesData](msg)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
		return
	}
	if listed.Error != "" {
		if !writeMissingCommand(w, listed.Error, "the journal can't be read") {
			servicehttp.WriteError(w, http.StatusBadRequest, listed.Error)
		}
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"entries": listed.Entries,
	})
}

func (d *Deps) ProcessAgentJournalEntries(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	data, err := agentmgr.Decode[agentmgr.JournalEntriesData](msg)
	if err != nil {
		log.Printf("agentws: invalid journal entries data: %v", err)
		return
	}
	deliverAgentReply(d.JournalReplies, matchPayloadID, agentmgr.MsgJournalEntries, conn, msg, data.RequestID)
}
