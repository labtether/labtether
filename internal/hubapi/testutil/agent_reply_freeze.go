package testutil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

// The checks below pin how each Hub request that sends one message to an
// agent and waits for one reply behaves today: what a timeout and a failed
// send answer, that a late or duplicate reply never blocks the agent's read
// loop (it runs under a per-device lock), which replies count as the answer,
// and which ID matches a reply to its request. Each package lists its own
// requests; a change to any of this must change those lists on purpose.

// ReplyDevice is the device every request is made for.
const ReplyDevice = "nas01"

// Reply variants: each is a reply that differs from the right one in one way.
const (
	ReplyWithoutConn     = "reply without a connection"
	ReplyFromOtherCase   = "reply from NAS01 (other letter case)"
	ReplyFromOtherDevice = "reply from nas02"
	ReplyOtherEnvelopeID = "reply with another envelope ID"
	ReplyNoEnvelopeID    = "reply with no envelope ID"
	ReplyOtherPayloadID  = "reply with another request_id in its payload"
)

const (
	replyMarker          = "from-agent"
	replyVariantMarker   = "from-variant"
	replyDuplicateMarker = "duplicate-reply"
	// The handlers write 504 "agent did not respond in time", but
	// servicehttp.WriteError replaces every 5xx text with this one, so this
	// is what the person sees.
	replyTimeoutAnswer    = "An internal error occurred."
	replyNotConnectedText = "agent is not connected"
)

// ReplySite is one request that waits for one agent reply.
type ReplySite struct {
	Name               string
	Method, Path, Body string
	Sends              string // message type the Hub sends
	ReplyType          string // message type the agent answers with
	// Reply builds the agent's answer. The answer must show marker in the
	// HTTP response body.
	Reply func(requestID, marker string) any
	// ByEnvelope: the reply is matched by its envelope ID, not by the
	// request_id in its payload.
	ByEnvelope bool
	Wait       time.Duration // how long the request waits by default
	// Accepts says, for each reply variant, whether it counts as the answer.
	// A variant left out is pinned by an older test.
	Accepts map[string]bool
	// OfflinePinnedBy names the older test that pins the answer for a device
	// whose agent is not connected; empty means RunReplyOffline checks it.
	OfflinePinnedBy string
	// Bind sets up the package dependencies for one request.
	Bind func(*testing.T, *agentmgr.AgentManager) ReplyHarness
}

// ReplyHarness is a site bound to one set of package dependencies.
type ReplyHarness struct {
	Handle     http.HandlerFunc
	Deliver    func(*agentmgr.AgentConn, agentmgr.Message)
	Pending    func() int // waiters still registered
	SetTimeout func(func(defaultTimeout time.Duration) time.Duration)
}

func (s ReplySite) serve(h ReplyHarness) <-chan *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(s.Method, s.Path, strings.NewReader(s.Body))
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		h.Handle(rec, req)
		done <- rec
	}()
	return done
}

func (s ReplySite) message(t testing.TB, envelopeID, payloadID, marker string) agentmgr.Message {
	t.Helper()
	data, err := json.Marshal(s.Reply(payloadID, marker))
	if err != nil {
		t.Fatalf("encode reply: %v", err)
	}
	return agentmgr.Message{Type: s.ReplyType, ID: envelopeID, Data: data}
}

// requestKey is the ID a reply must carry to answer the sent request.
func (s ReplySite) requestKey(t testing.TB, sent agentmgr.Message) string {
	t.Helper()
	if sent.Type != s.Sends {
		t.Fatalf("Hub sent %q, want %q", sent.Type, s.Sends)
	}
	key := sent.ID
	if !s.ByEnvelope {
		var payload struct {
			RequestID string `json:"request_id"`
		}
		_ = json.Unmarshal(sent.Data, &payload)
		key = payload.RequestID
	}
	if key == "" {
		t.Fatalf("Hub sent no request ID: %+v", sent)
	}
	return key
}

// AwaitAnswer waits up to five seconds for a request to answer.
func AwaitAnswer(t testing.TB, done <-chan *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	t.Helper()
	select {
	case rec := <-done:
		return rec
	case <-time.After(5 * time.Second):
		t.Fatal("request did not answer within 5s")
	}
	return nil
}

// DeliverPromptly fails the test when deliver does not return within two
// seconds: a blocked delivery stalls every message from that agent.
func DeliverPromptly(t testing.TB, what string, deliver func()) {
	t.Helper()
	returned := make(chan struct{})
	go func() {
		defer close(returned)
		deliver()
	}()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatalf("delivering the %s blocked", what)
	}
}

func wantErrorAnswer(t testing.TB, rec *httptest.ResponseRecorder, code int, text string) {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != code || body.Error != text {
		t.Fatalf("answer %d %s, want %d %q", rec.Code, rec.Body.String(), code, text)
	}
}

func wantNoWaiters(t testing.TB, h ReplyHarness) {
	t.Helper()
	if n := h.Pending(); n != 0 {
		t.Fatalf("%d waiter(s) left registered after the request ended", n)
	}
}
