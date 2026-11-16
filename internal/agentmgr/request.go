package agentmgr

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// RequestErrorKind names why Request returned without a reply. Callers map it
// to their own answer; Request never writes HTTP or logs.
type RequestErrorKind string

const (
	// RequestInvalid: the call is incomplete (no ID, sender or timeout) or the
	// payload failed to encode. Nothing was sent.
	RequestInvalid RequestErrorKind = "invalid"
	// RequestBusy: the registry's admission limit is reached. Nothing was sent.
	RequestBusy RequestErrorKind = "busy"
	// RequestDuplicate: a key is already waiting. Nothing was sent.
	RequestDuplicate RequestErrorKind = "duplicate"
	// RequestSendFailed: Send returned an error; Unwrap gives it.
	RequestSendFailed RequestErrorKind = "send_failed"
	// RequestTimedOut: the timer fired before a reply.
	RequestTimedOut RequestErrorKind = "timed_out"
	// RequestCanceled: the context ended; Unwrap gives ctx.Err(), so
	// errors.Is(err, context.DeadlineExceeded) tells a deadline from a cancel.
	RequestCanceled RequestErrorKind = "canceled"
	// RequestDisconnected: the Disconnect channel closed before a reply.
	RequestDisconnected RequestErrorKind = "disconnected"
)

// RequestError is the only error type Request returns.
type RequestError struct {
	Kind RequestErrorKind
	ID   string
	Err  error
}

func (e *RequestError) Error() string {
	if e == nil {
		return "agent request failed"
	}
	if e.Err != nil {
		return "agent request " + string(e.Kind) + ": " + e.Err.Error()
	}
	return "agent request " + string(e.Kind)
}

func (e *RequestError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// RequestKind returns the kind of a Request error, or "" for any other error.
func RequestKind(err error) RequestErrorKind {
	var requestErr *RequestError
	if errors.As(err, &requestErr) && requestErr != nil {
		return requestErr.Kind
	}
	return ""
}

// Call describes one request. Rules that differ between existing callers are
// explicit fields; the zero value of each keeps the narrowest behaviour.
type Call[Req, Resp any] struct {
	// Type is the message type sent to the agent.
	Type string
	// ID is the request ID. It is the envelope ID of the sent message and the
	// waiter's key. The caller puts it in Payload too when the type needs it.
	ID string
	// Reserve lists extra keys held for uniqueness while the request runs
	// (the package preview's logical job key). They live in their own
	// namespace: a reply never reaches them and they never clash with an ID.
	Reserve []string
	Payload Req
	// Send writes the message. It is the only way Request reaches the agent;
	// tests pass a fake. Request does not check ctx before sending: use a
	// Send that calls SendContext for that.
	Send func(Message) error

	// Device is the device ID replies must come from, compared by the
	// handler's Match.Device. Empty skips the comparison.
	Device string
	// ExactConn, when set, refuses a reply from any other connection value.
	ExactConn *AgentConn
	// Accept checks the decoded reply against the request (for example the
	// echoed asset or endpoint). A refused reply leaves the waiter
	// registered. It runs in the agent's read loop and must not block.
	Accept func(Resp) bool

	// Timeout bounds the wait after a successful send.
	Timeout time.Duration
	// TimeoutFunc, when set, replaces Timeout and is read once, after the
	// send succeeds and while the waiter is registered.
	TimeoutFunc func() time.Duration
	// Disconnect ends the wait with RequestDisconnected when it closes;
	// usually conn.Done(). A reply delivered before it closed still wins.
	// Nil means a disconnect is not watched.
	Disconnect <-chan struct{}
}

// Request registers a waiter in p, sends one message and waits for the reply
// that p.Deliver hands over, the timer, ctx or Disconnect. The waiter is
// always removed before Request returns. A nil ctx means no context.
func Request[Req, Resp any](ctx context.Context, p *Pending[Resp], call Call[Req, Resp]) (Resp, error) {
	var zero Resp
	fail := func(kind RequestErrorKind, err error) (Resp, error) {
		return zero, &RequestError{Kind: kind, ID: call.ID, Err: err}
	}
	if p == nil || call.ID == "" || call.Send == nil || (call.TimeoutFunc == nil && call.Timeout <= 0) {
		return fail(RequestInvalid, nil)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	data, err := json.Marshal(call.Payload)
	if err != nil {
		return fail(RequestInvalid, err)
	}

	w := &waiter[Resp]{
		device: call.Device,
		conn:   call.ExactConn,
		accept: call.Accept,
		ch:     make(chan Resp, 1),
	}
	if kind := p.register(call.ID, call.Reserve, w); kind != "" {
		return fail(kind, nil)
	}
	defer p.unregister(call.ID, call.Reserve, w)

	if err := call.Send(Message{Type: call.Type, ID: call.ID, Data: data}); err != nil {
		return fail(RequestSendFailed, err)
	}

	timeout := call.Timeout
	if call.TimeoutFunc != nil {
		timeout = call.TimeoutFunc()
	}
	if timeout <= 0 {
		return fail(RequestInvalid, nil)
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case reply := <-w.ch:
		return reply, nil
	case <-ctx.Done():
		return fail(RequestCanceled, ctx.Err())
	case <-call.Disconnect:
		// A reply that came in before the connection closed still answers:
		// an agent may reply and then disconnect, as a reboot does.
		select {
		case reply := <-w.ch:
			return reply, nil
		default:
		}
		return fail(RequestDisconnected, nil)
	case <-timer.C:
		return fail(RequestTimedOut, nil)
	}
}
