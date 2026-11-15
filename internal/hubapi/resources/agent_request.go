package resources

import (
	"context"
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/servicehttp"
)

// AgentReplies holds the device requests waiting for one agent reply. A
// registry is shared by every request and reply type that used one map
// before, so it holds the raw message and each request decodes its own type.
type AgentReplies = agentmgr.Pending[agentmgr.Message]

// How each reply type finds its waiting request. Every rule here is today's
// behaviour, pinned by agent_reply_freeze_test.go; some differ by history.
var (
	// Disk, cron and users skip the device check for a reply with no connection.
	matchListAcceptsNoConn = agentmgr.Match{Key: agentmgr.KeyPayload, Device: agentmgr.DeviceFold, AcceptNoConn: true}
	// Journal, service and network replies need a connection from the device.
	matchPayloadID = agentmgr.Match{Key: agentmgr.KeyPayload, Device: agentmgr.DeviceFold}
	// Process lists trim the request ID.
	matchProcessList = agentmgr.Match{Key: agentmgr.KeyPayload, TrimKey: true, Device: agentmgr.DeviceFold}
	// Process kill results carry no request ID; the envelope ID is the key.
	matchProcessKill = agentmgr.Match{Key: agentmgr.KeyEnvelope, TrimKey: true, Device: agentmgr.DeviceFold}
	// Package replies drop a non-empty envelope ID that differs from the payload ID.
	matchPackage = agentmgr.Match{Key: agentmgr.KeyPayloadEnvelopeIfSet, TrimKey: true, Device: agentmgr.DeviceFold}
)

// deliverAgentReply offers one decoded reply to the requests waiting in p.
func deliverAgentReply(p *AgentReplies, match agentmgr.Match, conn *agentmgr.AgentConn, msg agentmgr.Message, payloadID string) {
	p.Deliver(match, agentmgr.Reply[agentmgr.Message]{EnvelopeID: msg.ID, PayloadID: payloadID, Conn: conn, Value: msg})
}

// requestAgent sends one message on conn and waits in p for the reply from
// assetID. The wait is read after the send, through agentReplyTimeout.
func (d *Deps) requestAgent(p *AgentReplies, conn *agentmgr.AgentConn, msgType, requestID string, payload any, assetID string, defaultTimeout time.Duration) (agentmgr.Message, error) {
	return agentmgr.Request(context.Background(), p, agentmgr.Call[any, agentmgr.Message]{
		Type:        msgType,
		ID:          requestID,
		Payload:     payload,
		Send:        conn.Send,
		Device:      assetID,
		TimeoutFunc: func() time.Duration { return replyWait(d.agentReplyTimeout(defaultTimeout)) },
	})
}

// replyWait keeps a zero or negative wait firing at once, as time.After did.
func replyWait(wait time.Duration) time.Duration {
	if wait <= 0 {
		return time.Nanosecond
	}
	return wait
}

// writeAgentRequestFailure writes the answer for a request that got no reply:
// 504 when the agent did not answer in time, otherwise "agent is not
// connected" (the send failed; nothing else can happen without a context, a
// disconnect watch or an admission limit).
func (d *Deps) writeAgentRequestFailure(w http.ResponseWriter, assetID string, err error) {
	if agentmgr.RequestKind(err) == agentmgr.RequestTimedOut {
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
		return
	}
	d.writeAgentNotConnected(w, assetID)
}
