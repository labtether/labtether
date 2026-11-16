package desktop

import (
	"net/http"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubapi/testutil"
)

// These pin how the Remote View requests that send one message to the agent
// and wait for one reply behave (see testutil/agent_reply_freeze.go).
// Display list, diagnostic and clipboard check the replying device the same
// way: its own connection, with the exact device ID.

type depsHandler func(*Deps, http.ResponseWriter, *http.Request)
type depsDeliver func(*Deps, *agentmgr.AgentConn, agentmgr.Message)

func bindDeps(site testutil.ReplySite, handle depsHandler, deliver depsDeliver, waiters func(*Deps) interface{ Len() int }) testutil.ReplySite {
	site.Bind = func(_ *testing.T, manager *agentmgr.AgentManager) testutil.ReplyHarness {
		d := &Deps{
			AgentMgr:          manager,
			DisplayReplies:    &DisplayReplies{},
			DiagnosticReplies: &DiagnosticReplies{},
			ClipboardReplies:  &ClipboardReplies{},
			GenerateRequestID: shared.GenerateRequestID,
		}
		return testutil.ReplyHarness{
			Handle:     func(w http.ResponseWriter, r *http.Request) { handle(d, w, r) },
			Deliver:    func(conn *agentmgr.AgentConn, msg agentmgr.Message) { deliver(d, conn, msg) },
			Route:      func(conn *agentmgr.AgentConn, msg agentmgr.Message) { desktopReplyHandlers[msg.Type](d, conn, msg) },
			Pending:    func() int { return waiters(d).Len() },
			SetTimeout: func(pick func(time.Duration) time.Duration) { d.replyTimeout = pick },
		}
	}
	return site
}

// The reply handlers the agent router calls, by message type.
var desktopReplyHandlers = map[string]depsDeliver{
	agentmgr.MsgDesktopDisplays: (*Deps).ProcessAgentDesktopDisplays, agentmgr.MsgDesktopDiagnosed: (*Deps).ProcessAgentDesktopDiagnosed,
	agentmgr.MsgClipboardData: (*Deps).ProcessAgentClipboardData, agentmgr.MsgClipboardSetAck: (*Deps).ProcessAgentClipboardSetAck,
}

func desktopReplySites() []testutil.ReplySite {
	site := func(name, method, path, body, sends, reply string, wait time.Duration, accepts map[string]bool, offline string, build func(id, marker string) any) testutil.ReplySite {
		return testutil.ReplySite{Name: name, Method: method, Path: path, Body: body, Sends: sends, ReplyType: reply,
			Wait: wait, Accepts: accepts, OfflinePinnedBy: offline, Reply: build}
	}
	// Replies from nas02 are pinned for these by
	// TestProcessAgentDesktopDisplaysRejectsMismatchedSender and
	// TestProcessAgentClipboard{Data,SetAck}IgnoresUnexpectedAgent.
	displays := map[string]bool{
		testutil.ReplyWithoutConn: false, testutil.ReplyFromOtherCase: false,
		testutil.ReplyOtherEnvelopeID: true, testutil.ReplyNoEnvelopeID: true, testutil.ReplyOtherPayloadID: false,
	}
	// The diagnostic takes a reply only from the device's own connection.
	diagnostic := map[string]bool{
		testutil.ReplyWithoutConn: false, testutil.ReplyFromOtherCase: false, testutil.ReplyFromOtherDevice: false,
		testutil.ReplyOtherEnvelopeID: true, testutil.ReplyNoEnvelopeID: true, testutil.ReplyOtherPayloadID: false,
	}
	// Every site compares the device ID exactly, so other letter case is dropped.
	clipboard := map[string]bool{
		testutil.ReplyWithoutConn: false, testutil.ReplyFromOtherCase: false,
		testutil.ReplyOtherEnvelopeID: true, testutil.ReplyNoEnvelopeID: true, testutil.ReplyOtherPayloadID: false,
	}
	// TestRemoteViewRoutesSayAgentIsNotConnected covers displays and
	// clipboard/get; clipboard/set takes the same connection check in
	// HandleClipboardRoutes before it picks the action.
	const offlineRemoteView = "TestRemoteViewRoutesSayAgentIsNotConnected"
	// Clipboard get and set share one registry; each takes only its own reply.
	clipboardData := func(id, m string) any { return agentmgr.ClipboardDataPayload{RequestID: id, Format: "text", Error: m} }
	clipboardAck := func(id, m string) any { return agentmgr.ClipboardSetAckData{RequestID: id, Error: m} }
	withForeign := func(site testutil.ReplySite, replyType string, build func(id, marker string) any) testutil.ReplySite {
		site.Foreign = []testutil.ForeignReply{{Type: replyType, Reply: build}}
		return site
	}
	return []testutil.ReplySite{
		bindDeps(site("display list", http.MethodGet, "/api/v1/nodes/nas01/displays", "",
			agentmgr.MsgDesktopListDisplays, agentmgr.MsgDesktopDisplays, 10*time.Second, displays, offlineRemoteView,
			func(id, m string) any {
				return agentmgr.DisplayListData{RequestID: id, Displays: []agentmgr.DisplayInfo{{Name: m, Width: 1920, Height: 1080}}}
			}),
			(*Deps).HandleNodeSubRoutes, (*Deps).ProcessAgentDesktopDisplays, func(d *Deps) interface{ Len() int } { return d.DisplayReplies }),
		bindDeps(site("desktop diagnostic", http.MethodPost, "/desktop/diagnose/nas01", "",
			agentmgr.MsgDesktopDiagnose, agentmgr.MsgDesktopDiagnosed, 15*time.Second, diagnostic,
			"TestHandleDesktopDiagnoseRequestRejectsDisconnectedAgent",
			func(id, m string) any { return agentmgr.DesktopDiagnosticData{RequestID: id, DesktopUser: m} }),
			(*Deps).HandleDesktopDiagnoseRequest, (*Deps).ProcessAgentDesktopDiagnosed,
			func(d *Deps) interface{ Len() int } { return d.DiagnosticReplies }),
		bindDeps(withForeign(site("clipboard get", http.MethodPost, "/api/v1/nodes/nas01/clipboard/get", `{}`,
			agentmgr.MsgClipboardGet, agentmgr.MsgClipboardData, 10*time.Second, clipboard, offlineRemoteView,
			clipboardData), agentmgr.MsgClipboardSetAck, clipboardAck),
			(*Deps).HandleClipboardRoutes, (*Deps).ProcessAgentClipboardData, func(d *Deps) interface{ Len() int } { return d.ClipboardReplies }),
		bindDeps(withForeign(site("clipboard set", http.MethodPost, "/api/v1/nodes/nas01/clipboard/set", `{"format":"text","text":"x"}`,
			agentmgr.MsgClipboardSet, agentmgr.MsgClipboardSetAck, 10*time.Second, clipboard, offlineRemoteView,
			clipboardAck), agentmgr.MsgClipboardData, clipboardData),
			(*Deps).HandleClipboardRoutes, (*Deps).ProcessAgentClipboardSetAck, func(d *Deps) interface{ Len() int } { return d.ClipboardReplies }),
	}
}

// The display answer also asks whether this Hub's RDP gateway runs; point it
// at a closed loopback port so that answer comes at once.
func noRDPGateway(t *testing.T) { rdpGatewayOnLoopback(t, loopbackPortWithNoListener(t)) }

func TestDesktopAgentRequestTimeouts(t *testing.T) {
	noRDPGateway(t)
	testutil.RunReplyTimeouts(t, desktopReplySites())
}

func TestDesktopAgentRequestSendFailures(t *testing.T) {
	noRDPGateway(t)
	testutil.RunReplySendFailures(t, desktopReplySites())
}

func TestDesktopAgentRequestDuplicateReplies(t *testing.T) {
	noRDPGateway(t)
	testutil.RunReplyDuplicates(t, desktopReplySites())
}

func TestDesktopAgentRequestReplyMatching(t *testing.T) {
	noRDPGateway(t)
	testutil.RunReplyVariants(t, desktopReplySites())
}

func TestDesktopAgentRequestDropsRepliesOfAnotherType(t *testing.T) {
	testutil.RunReplyForeignTypes(t, desktopReplySites())
}

func TestDesktopAgentRequestEndsWhenTheAgentDisconnects(t *testing.T) {
	noRDPGateway(t)
	testutil.RunReplyDisconnects(t, desktopReplySites())
}
