package desktop

import (
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/servicehttp"
)

// Registries of Remote View requests waiting for one agent reply. Clipboard
// get and set share one, so it holds the raw message and each request
// decodes its own type.
type (
	DisplayReplies    = agentmgr.Pending[agentmgr.DisplayListData]
	DiagnosticReplies = agentmgr.Pending[agentmgr.DesktopDiagnosticData]
	ClipboardReplies  = agentmgr.Pending[agentmgr.Message]
)

// How each reply finds its waiting request: today's behaviour, pinned by
// agent_reply_freeze_test.go. Each checks the replying device differently.
var (
	matchDisplays = agentmgr.Match{Key: agentmgr.KeyPayload, Device: agentmgr.DeviceFold}
	// The diagnostic checks no device at all.
	matchDiagnostic = agentmgr.Match{Key: agentmgr.KeyPayload, Device: agentmgr.DeviceAny}
	// Clipboard compares the trimmed device ID exactly, so letter case counts.
	matchClipboard = agentmgr.Match{Key: agentmgr.KeyPayload, Device: agentmgr.DeviceExactTrim}
)

// replyWaitFunc reads the wait after the send through agentReplyTimeout, and
// keeps a zero or negative wait firing at once, as time.After did.
func (d *Deps) replyWaitFunc(defaultTimeout time.Duration) func() time.Duration {
	return func() time.Duration {
		if wait := d.agentReplyTimeout(defaultTimeout); wait > 0 {
			return wait
		}
		return time.Nanosecond
	}
}

// writeAgentRequestFailure writes the answer for a request that got no reply:
// 504 when the agent did not answer in time, otherwise "agent is not
// connected" (the send failed).
func (d *Deps) writeAgentRequestFailure(w http.ResponseWriter, assetID string, err error) {
	if agentmgr.RequestKind(err) == agentmgr.RequestTimedOut {
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
		return
	}
	d.writeAgentNotConnected(w, assetID)
}
