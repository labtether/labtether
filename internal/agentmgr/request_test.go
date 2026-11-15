package agentmgr

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// Fault: two replies racing in from the read loop both count as delivered,
// or the request returns a reply other than the one Deliver accepted.
func TestRequestTakesAtMostOneReplyUnderConcurrentReplies(t *testing.T) {
	p := &Pending[testReply]{}
	rec := newRecorder()
	done := start(t, context.Background(), p, baseCall("req-1", rec), rec)

	const senders = 32
	results := make(chan string, senders)
	ready := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < senders; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-ready
			value := fmt.Sprintf("reply-%d", i)
			if p.Deliver(anyDevice, payloadReply("req-1", value, nil)) {
				results <- value
			}
		}(i)
	}
	close(ready)
	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(guard):
		t.Fatal("a concurrent Deliver blocked")
	}
	close(results)
	var winners []string
	for value := range results {
		winners = append(winners, value)
	}
	if len(winners) != 1 {
		t.Fatalf("%d replies reported delivered, want exactly 1: %v", len(winners), winners)
	}
	got := wait(t, done)
	if got.err != nil || got.reply.Value != winners[0] {
		t.Fatalf("request returned %+v, want the delivered reply %q", got, winners[0])
	}
}

// Fault: a request that ends any way other than a reply leaves its waiter
// registered, or keeps its admission slot, so the registry grows or locks up.
func TestRequestLeavesNoWaiterOnAnyEnding(t *testing.T) {
	cases := []struct {
		name string
		kind RequestErrorKind
		run  func(t *testing.T, p *Pending[testReply]) outcome
	}{
		{name: "success", run: func(t *testing.T, p *Pending[testReply]) outcome {
			rec := newRecorder()
			done := start(t, context.Background(), p, baseCall("req-ok", rec), rec)
			deliverWithin(t, p, anyDevice, payloadReply("req-ok", "fine", nil))
			return wait(t, done)
		}},
		{name: "timeout", kind: RequestTimedOut, run: func(t *testing.T, p *Pending[testReply]) outcome {
			rec := newRecorder()
			call := baseCall("req-timeout", rec)
			call.Timeout = 10 * time.Millisecond
			return wait(t, start(t, context.Background(), p, call, rec))
		}},
		{name: "cancel", kind: RequestCanceled, run: func(t *testing.T, p *Pending[testReply]) outcome {
			rec := newRecorder()
			ctx, cancel := context.WithCancel(context.Background())
			done := start(t, ctx, p, baseCall("req-cancel", rec), rec)
			cancel()
			return wait(t, done)
		}},
		{name: "send failure", kind: RequestSendFailed, run: func(t *testing.T, p *Pending[testReply]) outcome {
			rec := newRecorder()
			rec.err = errors.New("socket closed")
			return wait(t, start(t, context.Background(), p, baseCall("req-send", rec), rec))
		}},
		{name: "disconnect", kind: RequestDisconnected, run: func(t *testing.T, p *Pending[testReply]) outcome {
			rec := newRecorder()
			gone := make(chan struct{})
			call := baseCall("req-gone", rec)
			call.Disconnect = gone
			call.Reserve = []string{"logical-gone"}
			done := start(t, context.Background(), p, call, rec)
			close(gone)
			return wait(t, done)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewPending[testReply](1)
			got := tc.run(t, p)
			if kind := RequestKind(got.err); kind != tc.kind {
				t.Fatalf("ended with kind %q (%v), want %q", kind, got.err, tc.kind)
			}
			if n := p.Len(); n != 0 {
				t.Fatalf("%d waiter(s) left registered", n)
			}
			rec := newRecorder()
			call := baseCall("req-next", rec)
			call.Timeout = time.Millisecond
			if _, err := Request(context.Background(), p, call); RequestKind(err) == RequestBusy {
				t.Fatal("admission slot was not released")
			}
		})
	}
}

// Fault: Deliver waits for the request to read, which would stall the
// agent's read loop under its dispatch lock.
func TestDeliverNeverBlocksWhenNobodyReads(t *testing.T) {
	p := &Pending[testReply]{}
	rec := newRecorder()
	var during []string
	rec.hook = func(Message) {
		// Request is inside Send here and not reading yet. This runs on the
		// request's goroutine, so it records instead of failing the test.
		for _, value := range []string{"first", "second", "third"} {
			result := make(chan bool, 1)
			go func() { result <- p.Deliver(anyDevice, payloadReply("req-1", value, nil)) }()
			select {
			case ok := <-result:
				during = append(during, fmt.Sprint(ok))
			case <-time.After(guard):
				during = append(during, "blocked")
			}
		}
	}
	done := start(t, context.Background(), p, baseCall("req-1", rec), rec)
	got := wait(t, done)
	if fmt.Sprint(during) != "[true false false]" {
		// "blocked" means a Deliver waited for a reader.
		t.Fatalf("delivered while unread: %v, want [true false false]", during)
	}
	if got.err != nil || got.reply.Value != "first" {
		t.Fatalf("request returned %+v, want the first reply", got)
	}
	if deliverWithin(t, p, anyDevice, payloadReply("req-1", "late", nil)) {
		t.Fatal("a reply after the request ended was delivered")
	}
}

// Fault: the waiting rules map to the wrong error kind, hiding the cause the
// caller turns into its own answer.
func TestRequestEndsWithTypedErrors(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		rec := newRecorder()
		call := baseCall("req-1", rec)
		call.Timeout = 10 * time.Millisecond
		got := wait(t, start(t, context.Background(), &Pending[testReply]{}, call, rec))
		if RequestKind(got.err) != RequestTimedOut {
			t.Fatalf("got %v, want timed out", got.err)
		}
	})
	t.Run("cancel", func(t *testing.T) {
		rec := newRecorder()
		ctx, cancel := context.WithCancel(context.Background())
		done := start(t, ctx, &Pending[testReply]{}, baseCall("req-1", rec), rec)
		cancel()
		got := wait(t, done)
		if RequestKind(got.err) != RequestCanceled || !errors.Is(got.err, context.Canceled) {
			t.Fatalf("got %v, want canceled wrapping context.Canceled", got.err)
		}
	})
	t.Run("deadline", func(t *testing.T) {
		rec := newRecorder()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		got := wait(t, start(t, ctx, &Pending[testReply]{}, baseCall("req-1", rec), rec))
		if RequestKind(got.err) != RequestCanceled || !errors.Is(got.err, context.DeadlineExceeded) {
			t.Fatalf("got %v, want canceled wrapping context.DeadlineExceeded", got.err)
		}
	})
	t.Run("send failure", func(t *testing.T) {
		rec := newRecorder()
		sendErr := errors.New("socket closed")
		rec.err = sendErr
		got := wait(t, start(t, context.Background(), &Pending[testReply]{}, baseCall("req-1", rec), rec))
		if RequestKind(got.err) != RequestSendFailed || !errors.Is(got.err, sendErr) {
			t.Fatalf("got %v, want send failed wrapping the send error", got.err)
		}
	})
	t.Run("disconnect watched", func(t *testing.T) {
		rec := newRecorder()
		gone := make(chan struct{})
		call := baseCall("req-1", rec)
		call.Disconnect = gone
		done := start(t, context.Background(), &Pending[testReply]{}, call, rec)
		close(gone)
		if got := wait(t, done); RequestKind(got.err) != RequestDisconnected {
			t.Fatalf("got %v, want disconnected", got.err)
		}
	})
	t.Run("disconnect not watched", func(t *testing.T) {
		rec := newRecorder()
		conn := &AgentConn{AssetID: "nas01"}
		conn.Close()
		call := baseCall("req-1", rec)
		call.ExactConn = conn
		call.Timeout = 10 * time.Millisecond
		if got := wait(t, start(t, context.Background(), &Pending[testReply]{}, call, rec)); RequestKind(got.err) != RequestTimedOut {
			t.Fatalf("got %v, want timed out: a request without Disconnect must not notice it", got.err)
		}
	})
}

// Fault: an incomplete call or a payload that cannot be encoded still sends.
func TestRequestRefusesInvalidCallsWithoutSending(t *testing.T) {
	cases := map[string]func(*Call[testRequest, testReply]){
		"no id":      func(c *Call[testRequest, testReply]) { c.ID = "" },
		"no sender":  func(c *Call[testRequest, testReply]) { c.Send = nil },
		"no timeout": func(c *Call[testRequest, testReply]) { c.Timeout = 0 },
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			rec := newRecorder()
			call := baseCall("req-1", rec)
			edit(&call)
			if _, err := Request(context.Background(), &Pending[testReply]{}, call); RequestKind(err) != RequestInvalid {
				t.Fatalf("got %v, want invalid", err)
			}
			if rec.count() != 0 {
				t.Fatal("an invalid call was sent")
			}
		})
	}
	t.Run("payload does not encode", func(t *testing.T) {
		sent := 0
		call := Call[func(), testReply]{Type: "test.request", ID: "req-1", Payload: func() {}, Timeout: guard,
			Send: func(Message) error { sent++; return nil }}
		if _, err := Request(context.Background(), &Pending[testReply]{}, call); RequestKind(err) != RequestInvalid {
			t.Fatalf("got %v, want invalid", err)
		}
		if sent != 0 {
			t.Fatal("an unencodable payload was sent")
		}
	})
}

// Fault: the timeout is read before the send. The step-two freeze tests hold
// a request inside its timeout lookup to make "waiter registered, reply not
// yet read" a fixed state; reading it earlier would break that hook.
func TestRequestReadsTimeoutFuncAfterSendWithWaiterRegistered(t *testing.T) {
	p := &Pending[testReply]{}
	var order []string
	call := baseCall("req-1", nil)
	call.Send = func(Message) error { order = append(order, "send"); return nil }
	call.Timeout = 0
	call.TimeoutFunc = func() time.Duration {
		order = append(order, fmt.Sprintf("timeout(waiters=%d)", p.Len()))
		return 10 * time.Millisecond
	}
	if _, err := Request(context.Background(), p, call); RequestKind(err) != RequestTimedOut {
		t.Fatalf("got %v, want timed out", err)
	}
	if fmt.Sprint(order) != "[send timeout(waiters=1)]" {
		t.Fatalf("order %v, want [send timeout(waiters=1)]", order)
	}
}

// Fault: the admission bound lets more requests wait than the limit (power
// control allows 128), or a refused request still reaches the agent.
func TestRequestAdmissionBound(t *testing.T) {
	p := NewPending[testReply](1)
	_, stop := waitOn(t, p, baseCall("req-1", nil))

	rec := newRecorder()
	if _, err := Request(context.Background(), p, baseCall("req-2", rec)); RequestKind(err) != RequestBusy {
		t.Fatalf("second request got %v, want busy", err)
	}
	if rec.count() != 0 {
		t.Fatal("a refused request was sent")
	}
	stop()

	rec = newRecorder()
	call := baseCall("req-3", rec)
	call.Timeout = time.Millisecond
	if _, err := Request(context.Background(), p, call); RequestKind(err) != RequestTimedOut {
		t.Fatalf("request after release got %v, want admitted (timed out)", err)
	}
}

// Fault: a second request with a key already waiting replaces the first
// waiter (today's sync.Map Store), so the first reply goes to the wrong one.
func TestRequestRefusesKeyAlreadyWaiting(t *testing.T) {
	p := &Pending[testReply]{}
	first := baseCall("req-1", nil)
	first.Reserve = []string{"logical-1"}
	done, _ := waitOn(t, p, first)

	for name, call := range map[string]Call[testRequest, testReply]{
		"same id": baseCall("req-1", nil),
		"same reserved key": func() Call[testRequest, testReply] {
			c := baseCall("req-2", nil)
			c.Reserve = []string{"logical-1"}
			return c
		}(),
		"reserved twice": func() Call[testRequest, testReply] {
			c := baseCall("req-3", nil)
			c.Reserve = []string{"x", "x"}
			return c
		}(),
	} {
		rec := newRecorder()
		call.Send = rec.send
		if _, err := Request(context.Background(), p, call); RequestKind(err) != RequestDuplicate {
			t.Fatalf("%s: got %v, want duplicate", name, err)
		}
		if rec.count() != 0 {
			t.Fatalf("%s: a duplicate request was sent", name)
		}
	}
	if !deliverWithin(t, p, anyDevice, payloadReply("req-1", "for-first", nil)) {
		t.Fatal("the first waiter lost its reply")
	}
	if got := wait(t, done); got.reply.Value != "for-first" {
		t.Fatalf("first request returned %+v", got)
	}
}

// Fault: a reply reaches a request other than the one its key names.
func TestReplyForAnotherRequestIsNotDelivered(t *testing.T) {
	p := &Pending[testReply]{}
	doneA, stopA := waitOn(t, p, baseCall("req-a", nil))
	doneB, _ := waitOn(t, p, baseCall("req-b", nil))

	if !deliverWithin(t, p, anyDevice, payloadReply("req-b", "for-b", nil)) {
		t.Fatal("reply for b was not delivered")
	}
	if got := wait(t, doneB); got.reply.Value != "for-b" {
		t.Fatalf("b returned %+v", got)
	}
	if deliverWithin(t, p, anyDevice, payloadReply("req-c", "for-c", nil)) {
		t.Fatal("a reply for an unknown request was delivered")
	}
	select {
	case got := <-doneA:
		t.Fatalf("a finished with another request's reply: %+v", got)
	default:
	}
	if got := stopA(); RequestKind(got.err) != RequestCanceled {
		t.Fatalf("a ended with %+v, want only its own cancel", got)
	}
}
