package desktop

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubapi/testutil"
)

// These pin today's behaviour of the Remote View requests that send one
// message to the agent and wait for one reply (see
// testutil/agent_reply_freeze.go). Display list, diagnostic and clipboard
// each check the replying device differently; a shared helper must keep that.

type depsHandler func(*Deps, http.ResponseWriter, *http.Request)
type depsDeliver func(*Deps, *agentmgr.AgentConn, agentmgr.Message)

func bindDeps(site testutil.ReplySite, handle depsHandler, deliver depsDeliver, waiters func(*Deps) *sync.Map) testutil.ReplySite {
	site.Bind = func(_ *testing.T, manager *agentmgr.AgentManager) testutil.ReplyHarness {
		d := &Deps{
			AgentMgr:                 manager,
			DisplayBridges:           &sync.Map{},
			DesktopDiagnosticWaiters: &sync.Map{},
			ClipboardBridges:         &sync.Map{},
			GenerateRequestID:        shared.GenerateRequestID,
		}
		return testutil.ReplyHarness{
			Handle:  func(w http.ResponseWriter, r *http.Request) { handle(d, w, r) },
			Deliver: func(conn *agentmgr.AgentConn, msg agentmgr.Message) { deliver(d, conn, msg) },
			Pending: func() int {
				n := 0
				waiters(d).Range(func(any, any) bool { n++; return true })
				return n
			},
			SetTimeout: func(pick func(time.Duration) time.Duration) { d.replyTimeout = pick },
		}
	}
	return site
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
		testutil.ReplyWithoutConn: false, testutil.ReplyFromOtherCase: true,
		testutil.ReplyOtherEnvelopeID: true, testutil.ReplyNoEnvelopeID: true, testutil.ReplyOtherPayloadID: false,
	}
	// The diagnostic checks no device at all.
	diagnostic := map[string]bool{
		testutil.ReplyWithoutConn: true, testutil.ReplyFromOtherCase: true, testutil.ReplyFromOtherDevice: true,
		testutil.ReplyOtherEnvelopeID: true, testutil.ReplyNoEnvelopeID: true, testutil.ReplyOtherPayloadID: false,
	}
	// Clipboard compares the device ID exactly, so other letter case is dropped.
	clipboard := map[string]bool{
		testutil.ReplyWithoutConn: false, testutil.ReplyFromOtherCase: false,
		testutil.ReplyOtherEnvelopeID: true, testutil.ReplyNoEnvelopeID: true, testutil.ReplyOtherPayloadID: false,
	}
	// TestRemoteViewRoutesSayAgentIsNotConnected covers displays and
	// clipboard/get; clipboard/set takes the same connection check in
	// HandleClipboardRoutes before it picks the action.
	const offlineRemoteView = "TestRemoteViewRoutesSayAgentIsNotConnected"
	return []testutil.ReplySite{
		bindDeps(site("display list", http.MethodGet, "/api/v1/nodes/nas01/displays", "",
			agentmgr.MsgDesktopListDisplays, agentmgr.MsgDesktopDisplays, 10*time.Second, displays, offlineRemoteView,
			func(id, m string) any {
				return agentmgr.DisplayListData{RequestID: id, Displays: []agentmgr.DisplayInfo{{Name: m, Width: 1920, Height: 1080}}}
			}),
			(*Deps).HandleNodeSubRoutes, (*Deps).ProcessAgentDesktopDisplays, func(d *Deps) *sync.Map { return d.DisplayBridges }),
		bindDeps(site("desktop diagnostic", http.MethodPost, "/desktop/diagnose/nas01", "",
			agentmgr.MsgDesktopDiagnose, agentmgr.MsgDesktopDiagnosed, 15*time.Second, diagnostic,
			"TestHandleDesktopDiagnoseRequestRejectsDisconnectedAgent",
			func(id, m string) any { return agentmgr.DesktopDiagnosticData{RequestID: id, DesktopUser: m} }),
			(*Deps).HandleDesktopDiagnoseRequest, (*Deps).ProcessAgentDesktopDiagnosed,
			func(d *Deps) *sync.Map { return d.DesktopDiagnosticWaiters }),
		bindDeps(site("clipboard get", http.MethodPost, "/api/v1/nodes/nas01/clipboard/get", `{}`,
			agentmgr.MsgClipboardGet, agentmgr.MsgClipboardData, 10*time.Second, clipboard, offlineRemoteView,
			func(id, m string) any { return agentmgr.ClipboardDataPayload{RequestID: id, Format: "text", Error: m} }),
			(*Deps).HandleClipboardRoutes, (*Deps).ProcessAgentClipboardData, func(d *Deps) *sync.Map { return d.ClipboardBridges }),
		bindDeps(site("clipboard set", http.MethodPost, "/api/v1/nodes/nas01/clipboard/set", `{"format":"text","text":"x"}`,
			agentmgr.MsgClipboardSet, agentmgr.MsgClipboardSetAck, 10*time.Second, clipboard, offlineRemoteView,
			func(id, m string) any { return agentmgr.ClipboardSetAckData{RequestID: id, Error: m} }),
			(*Deps).HandleClipboardRoutes, (*Deps).ProcessAgentClipboardSetAck, func(d *Deps) *sync.Map { return d.ClipboardBridges }),
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
