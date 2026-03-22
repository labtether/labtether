package desktop

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/servicehttp"
)

const desktopDiagnoseTimeout = 15 * time.Second

// HandleDesktopDiagnoseRequest handles POST /desktop/diagnose/{assetID}.
// It sends a desktop.diagnose message to the connected agent and waits for
// the desktop.diagnosed response, returning the full diagnostic payload.
func (d *Deps) HandleDesktopDiagnoseRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	assetID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/desktop/diagnose/"))
	if assetID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset id required")
		return
	}

	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "agent not connected")
		return
	}

	requestID := d.GenerateRequestID()
	resultCh := make(chan agentmgr.DesktopDiagnosticData, 1)
	d.DesktopDiagnosticWaiters.Store(requestID, resultCh)
	defer d.DesktopDiagnosticWaiters.Delete(requestID)

	payload, _ := json.Marshal(agentmgr.DesktopDiagnosticRequest{RequestID: requestID})
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgDesktopDiagnose,
		ID:   requestID,
		Data: payload,
	}); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to send diagnose request to agent")
		return
	}

	select {
	case result := <-resultCh:
		servicehttp.WriteJSON(w, http.StatusOK, result)
	case <-time.After(desktopDiagnoseTimeout):
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
	}
}

// ProcessAgentDesktopDiagnosed handles desktop.diagnosed messages from the agent.
func (d *Deps) ProcessAgentDesktopDiagnosed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.DesktopDiagnosticData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("desktop_diagnose: invalid desktop.diagnosed payload: %v", err)
		return
	}

	raw, ok := d.DesktopDiagnosticWaiters.Load(data.RequestID)
	if !ok {
		return
	}
	ch, ok := raw.(chan agentmgr.DesktopDiagnosticData)
	if !ok || ch == nil {
		return
	}
	select {
	case ch <- data:
	default:
	}
}
