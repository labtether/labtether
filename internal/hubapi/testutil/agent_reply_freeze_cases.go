package testutil

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

// RunReplyTimeouts: with no reply, each request asks for its own default wait
// and answers 504 "agent did not respond in time". A reply that comes after
// that is dropped without blocking, and no waiter is left behind.
func RunReplyTimeouts(t *testing.T, sites []ReplySite) {
	for _, site := range sites {
		t.Run(site.Name, func(t *testing.T) {
			peer := NewAgentPeer(t, ReplyDevice)
			h := site.Bind(t, peer.Manager)
			var asked time.Duration
			h.SetTimeout(func(defaultTimeout time.Duration) time.Duration {
				asked = defaultTimeout
				return 10 * time.Millisecond
			})
			done := site.serve(h)
			key := site.requestKey(t, peer.Next(t))
			rec := AwaitAnswer(t, done)
			wantErrorAnswer(t, rec, http.StatusGatewayTimeout, replyTimeoutAnswer)
			if asked != site.Wait {
				t.Fatalf("request waited by default %v, want %v", asked, site.Wait)
			}
			DeliverPromptly(t, "late reply", func() { h.Deliver(peer.Conn, site.message(t, key, key, replyMarker)) })
			wantNoWaiters(t, h)
		})
	}
}

// RunReplySendFailures: when the agent is registered but the send fails, the
// request answers 409 "agent is not connected", as for an offline agent.
func RunReplySendFailures(t *testing.T, sites []ReplySite) {
	for _, site := range sites {
		t.Run(site.Name, func(t *testing.T) {
			peer := NewAgentPeer(t, ReplyDevice)
			peer.Conn.Close() // stays registered; every send now fails
			h := site.Bind(t, peer.Manager)
			h.SetTimeout(func(time.Duration) time.Duration { return 10 * time.Millisecond })
			rec := AwaitAnswer(t, site.serve(h))
			wantErrorAnswer(t, rec, http.StatusConflict, replyNotConnectedText)
			wantNoWaiters(t, h)
		})
	}
}

// RunReplyOffline: with no agent registered, the request answers 409 "agent is
// not connected". Sites an older test already pins are skipped.
func RunReplyOffline(t *testing.T, sites []ReplySite) {
	for _, site := range sites {
		if site.OfflinePinnedBy != "" {
			continue
		}
		t.Run(site.Name, func(t *testing.T) {
			h := site.Bind(t, agentmgr.NewManager())
			h.SetTimeout(func(time.Duration) time.Duration { return 10 * time.Millisecond })
			rec := AwaitAnswer(t, site.serve(h))
			wantErrorAnswer(t, rec, http.StatusConflict, replyNotConnectedText)
		})
	}
}

// RunReplyDuplicates: while the request has not yet read its reply, more
// replies for it are dropped without blocking, and the first one is the
// answer. The request is held just before it starts waiting: its timeout is
// resolved there, after the send, while its waiter is registered.
func RunReplyDuplicates(t *testing.T, sites []ReplySite) {
	for _, site := range sites {
		t.Run(site.Name, func(t *testing.T) {
			peer := NewAgentPeer(t, ReplyDevice)
			h := site.Bind(t, peer.Manager)
			release := make(chan struct{})
			h.SetTimeout(func(time.Duration) time.Duration {
				<-release
				return 2 * time.Second
			})
			done := site.serve(h)
			key := site.requestKey(t, peer.Next(t))
			DeliverPromptly(t, "first reply", func() { h.Deliver(peer.Conn, site.message(t, key, key, replyMarker)) })
			for i := 0; i < 2; i++ {
				DeliverPromptly(t, "duplicate reply", func() {
					h.Deliver(peer.Conn, site.message(t, key, key, replyDuplicateMarker))
				})
			}
			close(release)
			body := AwaitAnswer(t, done).Body.String()
			if !strings.Contains(body, replyMarker) || strings.Contains(body, replyDuplicateMarker) {
				t.Fatalf("answer %s, want the first reply", body)
			}
			wantNoWaiters(t, h)
		})
	}
}

type replyVariant struct {
	conn                func(hub *agentmgr.AgentConn) *agentmgr.AgentConn
	envelope, payloadID func(key string) string
}

var replyVariants = map[string]replyVariant{
	ReplyWithoutConn:     {conn: func(*agentmgr.AgentConn) *agentmgr.AgentConn { return nil }},
	ReplyFromOtherCase:   {conn: func(*agentmgr.AgentConn) *agentmgr.AgentConn { return &agentmgr.AgentConn{AssetID: "NAS01"} }},
	ReplyFromOtherDevice: {conn: func(*agentmgr.AgentConn) *agentmgr.AgentConn { return &agentmgr.AgentConn{AssetID: "nas02"} }},
	ReplyOtherEnvelopeID: {envelope: func(string) string { return "other-request" }},
	ReplyNoEnvelopeID:    {envelope: func(string) string { return "" }},
	ReplyOtherPayloadID:  {payloadID: func(string) string { return "other-request" }},
}

// RunReplyVariants: for each variant a site lists, the variant reply comes
// first and the right reply second. The answer shows which one the request
// took, so a variant that is accepted today must stay accepted and one that
// is dropped must stay dropped.
func RunReplyVariants(t *testing.T, sites []ReplySite) {
	for _, site := range sites {
		t.Run(site.Name, func(t *testing.T) {
			peer := NewAgentPeer(t, ReplyDevice)
			h := site.Bind(t, peer.Manager)
			h.SetTimeout(func(time.Duration) time.Duration { return 2 * time.Second })
			for name, accepted := range site.Accepts {
				variant, ok := replyVariants[name]
				if !ok {
					t.Fatalf("unknown reply variant %q", name)
				}
				done := site.serve(h)
				key := site.requestKey(t, peer.Next(t))
				conn, envelope, payloadID := peer.Conn, key, key
				if variant.conn != nil {
					conn = variant.conn(peer.Conn)
				}
				if variant.envelope != nil {
					envelope = variant.envelope(key)
				}
				if variant.payloadID != nil {
					payloadID = variant.payloadID(key)
				}
				DeliverPromptly(t, name, func() { h.Deliver(conn, site.message(t, envelope, payloadID, replyVariantMarker)) })
				DeliverPromptly(t, "right reply", func() { h.Deliver(peer.Conn, site.message(t, key, key, replyMarker)) })
				body := AwaitAnswer(t, done).Body.String()
				switch {
				case accepted && !strings.Contains(body, replyVariantMarker):
					t.Errorf("%s: dropped, but it is accepted today; answer %s", name, body)
				case !accepted && !strings.Contains(body, replyMarker):
					t.Errorf("%s: accepted, but it is dropped today; answer %s", name, body)
				}
				wantNoWaiters(t, h)
			}
		})
	}
}

// RunReplyRouting: the right reply, handed to Deliver, answers the request.
// Callers bind Deliver to the live agent message router, so this checks that
// each reply type reaches the request waiting for it.
func RunReplyRouting(t *testing.T, sites []ReplySite) {
	for _, site := range sites {
		t.Run(site.Name, func(t *testing.T) {
			peer := NewAgentPeer(t, ReplyDevice)
			h := site.Bind(t, peer.Manager)
			done := site.serve(h)
			key := site.requestKey(t, peer.Next(t))
			DeliverPromptly(t, "reply", func() { h.Deliver(peer.Conn, site.message(t, key, key, replyMarker)) })
			if body := AwaitAnswer(t, done).Body.String(); !strings.Contains(body, replyMarker) {
				t.Fatalf("answer %s, want the agent's reply", body)
			}
			wantNoWaiters(t, h)
		})
	}
}
