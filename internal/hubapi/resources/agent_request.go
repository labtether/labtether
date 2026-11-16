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

// How each reply type finds its waiting request, pinned by
// agent_reply_freeze_test.go. The key rules differ by reply type. Every reply
// must come on a connection whose device ID equals the request's exactly:
// two devices may differ only in letter case.
var (
	// Disk, cron, users, journal, service and network replies carry the
	// request ID in the payload and need a connection from the device.
	matchPayloadID = agentmgr.Match{Key: agentmgr.KeyPayload, Device: agentmgr.DeviceExact}
	// Process lists trim the request ID.
	matchProcessList = agentmgr.Match{Key: agentmgr.KeyPayload, TrimKey: true, Device: agentmgr.DeviceExact}
	// Process kill results carry no request ID; the envelope ID is the key.
	matchProcessKill = agentmgr.Match{Key: agentmgr.KeyEnvelope, TrimKey: true, Device: agentmgr.DeviceExact}
	// Package replies drop a non-empty envelope ID that differs from the payload ID.
	matchPackage = agentmgr.Match{Key: agentmgr.KeyPayloadEnvelopeIfSet, TrimKey: true, Device: agentmgr.DeviceExact}
)

// agentReplyTypes names the one reply type each request takes. Several
// request types share a registry, so a waiter refuses every other type.
var agentReplyTypes = map[string]string{
	agentmgr.MsgDiskList:      agentmgr.MsgDiskListed,
	agentmgr.MsgCronList:      agentmgr.MsgCronListed,
	agentmgr.MsgUsersList:     agentmgr.MsgUsersListed,
	agentmgr.MsgJournalQuery:  agentmgr.MsgJournalEntries,
	agentmgr.MsgProcessList:   agentmgr.MsgProcessListed,
	agentmgr.MsgProcessKill:   agentmgr.MsgProcessKillResult,
	agentmgr.MsgServiceList:   agentmgr.MsgServiceListed,
	agentmgr.MsgServiceAction: agentmgr.MsgServiceResult,
	agentmgr.MsgNetworkList:   agentmgr.MsgNetworkListed,
	agentmgr.MsgNetworkAction: agentmgr.MsgNetworkResult,
	agentmgr.MsgPackageList:   agentmgr.MsgPackageListed,
	agentmgr.MsgPackageAction: agentmgr.MsgPackageResult,
}

// AcceptReplyTo is the Call.Accept for a request of requestType: it takes
// only that request's own reply type.
func AcceptReplyTo(requestType string) func(agentmgr.Message) bool {
	want := agentReplyTypes[requestType]
	return func(msg agentmgr.Message) bool { return want != "" && msg.Type == want }
}

// deliverAgentReply offers one decoded reply of replyType to the requests
// waiting in p. The router calls each handler for its own type only.
func deliverAgentReply(p *AgentReplies, match agentmgr.Match, replyType string, conn *agentmgr.AgentConn, msg agentmgr.Message, payloadID string) {
	msg.Type = replyType
	p.Deliver(match, agentmgr.Reply[agentmgr.Message]{EnvelopeID: msg.ID, PayloadID: payloadID, Conn: conn, Value: msg})
}

// requestAgent sends one message on conn and waits in p for the reply from
// assetID. The wait is read after the send, through agentReplyTimeout, and
// ends when conn closes: a reply can only come back on it.
func (d *Deps) requestAgent(p *AgentReplies, conn *agentmgr.AgentConn, msgType, requestID string, payload any, assetID string, defaultTimeout time.Duration) (agentmgr.Message, error) {
	return agentmgr.Request(context.Background(), p, agentmgr.Call[any, agentmgr.Message]{
		Type:        msgType,
		ID:          requestID,
		Payload:     payload,
		Send:        conn.Send,
		Device:      assetID,
		Accept:      AcceptReplyTo(msgType),
		TimeoutFunc: func() time.Duration { return replyWait(d.agentReplyTimeout(defaultTimeout)) },
		Disconnect:  conn.Done(),
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
// connected" (the send failed or the connection closed during the wait;
// nothing else can happen without a context or an admission limit).
func (d *Deps) writeAgentRequestFailure(w http.ResponseWriter, assetID string, err error) {
	if agentmgr.RequestKind(err) == agentmgr.RequestTimedOut {
		servicehttp.WriteAgentTimeout(w)
		return
	}
	d.writeAgentNotConnected(w, assetID)
}
