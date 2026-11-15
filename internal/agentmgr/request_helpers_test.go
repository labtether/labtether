package agentmgr

import (
	"context"
	"testing"
	"time"
)

// guard bounds every wait in these tests so a fault fails a test instead of
// hanging it. No test sleeps; each waits on a channel.
const guard = 2 * time.Second

type testReply struct {
	RequestID string `json:"request_id"`
	Value     string `json:"value"`
}

type testRequest struct {
	RequestID string `json:"request_id"`
}

type outcome struct {
	reply testReply
	err   error
}

// recorder is an injected sender. Each sent message goes to sent; an optional
// hook runs inside Send, while Request has not started waiting yet.
type recorder struct {
	sent chan Message
	err  error
	hook func(Message)
}

func newRecorder() *recorder { return &recorder{sent: make(chan Message, 16)} }

func (r *recorder) send(msg Message) error {
	r.sent <- msg
	if r.hook != nil {
		r.hook(msg)
	}
	return r.err
}

func (r *recorder) count() int { return len(r.sent) }

func baseCall(id string, rec *recorder) Call[testRequest, testReply] {
	return Call[testRequest, testReply]{
		Type:    "test.request",
		ID:      id,
		Payload: testRequest{RequestID: id},
		Send:    rec.send,
		Device:  "nas01",
		Timeout: guard,
	}
}

// start runs Request in a goroutine and returns once the message was sent,
// so the waiter is registered.
func start(t *testing.T, ctx context.Context, p *Pending[testReply], call Call[testRequest, testReply], rec *recorder) <-chan outcome {
	t.Helper()
	done := make(chan outcome, 1)
	go func() {
		reply, err := Request(ctx, p, call)
		done <- outcome{reply: reply, err: err}
	}()
	select {
	case <-rec.sent:
	case got := <-done:
		t.Fatalf("Request returned before sending: %+v", got)
	case <-time.After(guard):
		t.Fatal("Request sent nothing")
	}
	return done
}

// waitOn registers a waiter for call and returns a stop function that ends
// the request and returns its outcome.
func waitOn(t *testing.T, p *Pending[testReply], call Call[testRequest, testReply]) (<-chan outcome, func() outcome) {
	t.Helper()
	rec := newRecorder()
	call.Send = rec.send
	ctx, cancel := context.WithCancel(context.Background())
	done := start(t, ctx, p, call, rec)
	stop := func() outcome {
		cancel()
		return wait(t, done)
	}
	t.Cleanup(func() { cancel() })
	return done, stop
}

func wait(t *testing.T, done <-chan outcome) outcome {
	t.Helper()
	select {
	case got := <-done:
		return got
	case <-time.After(guard):
		t.Fatal("Request did not return")
		return outcome{}
	}
}

func payloadReply(id, value string, conn *AgentConn) Reply[testReply] {
	return Reply[testReply]{EnvelopeID: id, PayloadID: id, Conn: conn, Value: testReply{RequestID: id, Value: value}}
}

// deliverWithin runs Deliver and fails the test if it blocks.
func deliverWithin(t *testing.T, p *Pending[testReply], m Match, r Reply[testReply]) bool {
	t.Helper()
	result := make(chan bool, 1)
	go func() { result <- p.Deliver(m, r) }()
	select {
	case ok := <-result:
		return ok
	case <-time.After(guard):
		t.Fatal("Deliver blocked the read loop")
		return false
	}
}

var anyDevice = Match{Key: KeyPayload, Device: DeviceAny}
