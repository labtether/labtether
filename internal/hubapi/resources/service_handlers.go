package resources

import (
	"encoding/json"
	"errors"
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
		d.writeAgentNotConnected(w, assetID)
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
		d.writeAgentNotConnected(w, assetID)
		return
	}

	requestID := generateRequestID()
	msg, err := d.requestAgent(d.ServiceReplies, agentConn, agentmgr.MsgServiceList, requestID,
		agentmgr.ServiceListData{RequestID: requestID}, assetID, serviceListTimeout)
	if err != nil {
		d.writeAgentRequestFailure(w, assetID, err)
		return
	}
	var listed agentmgr.ServiceListedData
	if err := json.Unmarshal(msg.Data, &listed); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
		return
	}
	if listed.Error != "" {
		if !writeMissingCommand(w, listed.Error, "services can't be listed") {
			servicehttp.WriteError(w, http.StatusBadRequest, listed.Error)
		}
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, listed)
}

func (d *Deps) handleServiceAction(w http.ResponseWriter, r *http.Request, assetID, action string) {
	if !validServiceActions[action] {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid action: must be one of start, stop, restart, enable, disable")
		return
	}
	if !d.enforceAssetActionGuard(w, assetID) {
		return
	}

	if _, ok := d.AgentMgr.Get(assetID); !ok {
		d.writeAgentNotConnected(w, assetID)
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

	result, err := d.RunServiceAction(assetID, body.Service, action)
	switch {
	case errors.Is(err, ErrServiceAgentNotConnected):
		d.writeAgentNotConnected(w, assetID)
		return
	case errors.Is(err, ErrServiceAgentTimeout):
		servicehttp.WriteAgentTimeout(w)
		return
	case err != nil:
		servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
		return
	}
	if result.Error != "" {
		if !writeMissingCommand(w, result.Error, serviceActionWhat) {
			servicehttp.WriteError(w, http.StatusBadRequest, agentFailureMessage(result.Error, result.Output))
		}
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, result)
}

func (d *Deps) ProcessAgentServiceListed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.ServiceListedData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid service listed data: %v", err)
		return
	}
	deliverAgentReply(d.ServiceReplies, matchPayloadID, agentmgr.MsgServiceListed, conn, msg, data.RequestID)
}

func (d *Deps) ProcessAgentServiceResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.ServiceResultData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid service result data: %v", err)
		return
	}
	deliverAgentReply(d.ServiceReplies, matchPayloadID, agentmgr.MsgServiceResult, conn, msg, data.RequestID)
}
