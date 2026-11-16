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

// How each reply finds its waiting request, pinned by
// agent_reply_freeze_test.go. Every reply must come on a connection whose
// device ID equals the request's exactly: two devices may differ only in
// letter case.
var (
	matchDisplays   = agentmgr.Match{Key: agentmgr.KeyPayload, Device: agentmgr.DeviceExact}
	matchDiagnostic = agentmgr.Match{Key: agentmgr.KeyPayload, Device: agentmgr.DeviceExact}
	matchClipboard  = agentmgr.Match{Key: agentmgr.KeyPayload, Device: agentmgr.DeviceExact}
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
// connected" (the send failed or the connection closed during the wait).
func (d *Deps) writeAgentRequestFailure(w http.ResponseWriter, assetID string, err error) {
	if agentmgr.RequestKind(err) == agentmgr.RequestTimedOut {
		servicehttp.WriteAgentTimeout(w)
		return
	}
	d.writeAgentNotConnected(w, assetID)
}
