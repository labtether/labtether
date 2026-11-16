package desktop

import (
	"context"
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
	if !d.requireTargetAccess(w, r, assetID) {
		return
	}

	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		d.writeAgentNotConnected(w, assetID)
		return
	}
	if refuseUnadvertised(w, agentConn, agentmgr.MsgDesktopDiagnose) {
		return
	}

	requestID := d.GenerateRequestID()
	result, err := agentmgr.Request(context.Background(), d.DiagnosticReplies, agentmgr.Call[agentmgr.DesktopDiagnosticRequest, agentmgr.DesktopDiagnosticData]{
		Type:        agentmgr.MsgDesktopDiagnose,
		ID:          requestID,
		Payload:     agentmgr.DesktopDiagnosticRequest{RequestID: requestID},
		Send:        agentConn.Send,
		Device:      assetID,
		TimeoutFunc: d.replyWaitFunc(desktopDiagnoseTimeout),
		Disconnect:  agentConn.Done(),
	})
	if err != nil {
		d.writeAgentRequestFailure(w, assetID, err)
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, result)
}

// ProcessAgentDesktopDiagnosed handles desktop.diagnosed messages from the agent.
func (d *Deps) ProcessAgentDesktopDiagnosed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	data, err := agentmgr.Decode[agentmgr.DesktopDiagnosticData](msg)
	if err != nil {
		log.Printf("desktop_diagnose: invalid desktop.diagnosed payload: %v", err)
		return
	}

	d.DiagnosticReplies.Deliver(matchDiagnostic, agentmgr.Reply[agentmgr.DesktopDiagnosticData]{
		EnvelopeID: msg.ID, PayloadID: data.RequestID, Conn: conn, Value: data,
	})
}
