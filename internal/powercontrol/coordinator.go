package powercontrol

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/idgen"
)

const (
	DefaultActionTimeout        = 15 * time.Second
	DefaultMaxConcurrentActions = 128
)

// Sender is the narrow agent-manager surface used by the power coordinator.
// It is deliberately injectable so tests never execute a real power command.
// Get returns the asset's current connection; the action is sent on it.
type Sender interface {
	Get(assetID string) (*agentmgr.AgentConn, bool)
}

type ErrorKind string

const (
	ErrorInvalid       ErrorKind = "invalid_request"
	ErrorAgentOffline  ErrorKind = "agent_offline"
	ErrorSendFailed    ErrorKind = "send_failed"
	ErrorTimedOut      ErrorKind = "timed_out"
	ErrorCanceled      ErrorKind = "canceled"
	ErrorBusy          ErrorKind = "busy"
	ErrorUnsupported   ErrorKind = "unsupported"
	ErrorRejected      ErrorKind = "rejected"
	ErrorExecutionFail ErrorKind = "execution_failed"
)

// ActionError carries a stable error kind and the agent's closed result code.
// Message is safe, bounded operator-facing text from the typed result contract.
type ActionError struct {
	Kind      ErrorKind
	RequestID string
	Code      agentmgr.PowerResultCode
	Message   string
}

func (e *ActionError) Error() string {
	if e == nil {
		return "power action failed"
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return string(e.Kind)
}

func KindOf(err error) ErrorKind {
	var actionErr *ActionError
	if errors.As(err, &actionErr) {
		return actionErr.Kind
	}
	return ErrorExecutionFail
}

type Coordinator struct {
	sender  Sender
	timeout time.Duration
	// pending admits at most maxConcurrent actions, so one broad principal
	// cannot accumulate unbounded waiters by targeting many assets at once.
	pending *agentmgr.Pending[agentmgr.PowerResultData]
}

func New(sender Sender, timeout time.Duration) *Coordinator {
	return NewWithLimit(sender, timeout, DefaultMaxConcurrentActions)
}

// NewWithLimit constructs a coordinator with an explicit global concurrency
// bound. It is primarily useful for deterministic tests; production callers
// should use New so one broad principal cannot accumulate unbounded waiters by
// targeting many assets at once.
func NewWithLimit(sender Sender, timeout time.Duration, maxConcurrent int) *Coordinator {
	if timeout <= 0 {
		timeout = DefaultActionTimeout
	}
	if maxConcurrent <= 0 {
		maxConcurrent = DefaultMaxConcurrentActions
	}
	return &Coordinator{
		sender:  sender,
		timeout: timeout,
		pending: agentmgr.NewPending[agentmgr.PowerResultData](maxConcurrent),
	}
}

// Execute sends one typed action to exactly one connected agent and waits for
// a strictly correlated power.result from the connection it was sent on. It
// does not end when that connection closes: a device going down after a
// reboot or shut down closes it, so a missing result stays a timeout ("no
// confirmation"), never "not connected". Only an accepted result returns nil.
func (c *Coordinator) Execute(ctx context.Context, assetID string, action agentmgr.PowerAction) (agentmgr.PowerResultData, error) {
	assetID = strings.TrimSpace(assetID)
	if assetID == "" || len(assetID) > 256 || !action.Valid() {
		return agentmgr.PowerResultData{}, &ActionError{Kind: ErrorInvalid, Message: "invalid power action request"}
	}
	var conn *agentmgr.AgentConn
	if c != nil && c.sender != nil {
		conn, _ = c.sender.Get(assetID)
	}
	if conn == nil {
		return agentmgr.PowerResultData{}, &ActionError{Kind: ErrorAgentOffline, Message: "asset agent is not connected"}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	requestID := idgen.New("power")
	result, err := agentmgr.Request(ctx, c.pending, agentmgr.Call[agentmgr.PowerActionData, agentmgr.PowerResultData]{
		Type: agentmgr.MsgPowerAction,
		ID:   requestID,
		Payload: agentmgr.PowerActionData{
			RequestID: requestID,
			AssetID:   assetID,
			Action:    action,
		},
		Send:      conn.Send,
		Device:    assetID,
		ExactConn: conn,
		Accept: func(result agentmgr.PowerResultData) bool {
			return result.AssetID == assetID && result.Action == action
		},
		Timeout: c.timeout,
	})
	if err != nil {
		return requestFailure(err, requestID, assetID, action)
	}
	switch result.Status {
	case agentmgr.PowerResultAccepted:
		return result, nil
	case agentmgr.PowerResultUnsupported:
		return result, &ActionError{Kind: ErrorUnsupported, RequestID: requestID, Code: result.Code, Message: result.Message}
	case agentmgr.PowerResultRejected:
		return result, &ActionError{Kind: ErrorRejected, RequestID: requestID, Code: result.Code, Message: result.Message}
	case agentmgr.PowerResultFailed:
		return result, &ActionError{Kind: ErrorExecutionFail, RequestID: requestID, Code: result.Code, Message: result.Message}
	default:
		// HandleResult validates the closed enum before delivery, so this is a
		// defensive fail-closed guard rather than an expected path.
		return result, &ActionError{Kind: ErrorExecutionFail, RequestID: requestID, Message: "agent returned an invalid power result"}
	}
}

// requestFailure maps a shared request error to the power error contract.
func requestFailure(err error, requestID, assetID string, action agentmgr.PowerAction) (agentmgr.PowerResultData, error) {
	pendingResult := agentmgr.PowerResultData{RequestID: requestID, AssetID: assetID, Action: action}
	switch agentmgr.RequestKind(err) {
	case agentmgr.RequestBusy:
		return agentmgr.PowerResultData{AssetID: assetID, Action: action}, &ActionError{
			Kind:    ErrorBusy,
			Message: "too many power actions are already in progress",
		}
	case agentmgr.RequestInvalid:
		return agentmgr.PowerResultData{}, &ActionError{Kind: ErrorInvalid, RequestID: requestID, Message: "failed to encode power action"}
	case agentmgr.RequestSendFailed:
		return agentmgr.PowerResultData{}, &ActionError{Kind: ErrorSendFailed, RequestID: requestID, Message: "failed to deliver power action to agent"}
	case agentmgr.RequestCanceled:
		if errors.Is(err, context.DeadlineExceeded) {
			return pendingResult, &ActionError{Kind: ErrorTimedOut, RequestID: requestID, Message: "timed out waiting for the agent power result"}
		}
		return pendingResult, &ActionError{Kind: ErrorCanceled, RequestID: requestID, Message: "power action request canceled"}
	case agentmgr.RequestTimedOut:
		return pendingResult, &ActionError{Kind: ErrorTimedOut, RequestID: requestID, Message: "timed out waiting for the agent power result"}
	default:
		// A duplicate ID or a disconnect cannot happen here: power IDs are
		// unique and the coordinator does not watch the connection.
		return agentmgr.PowerResultData{}, &ActionError{Kind: ErrorExecutionFail, RequestID: requestID, Message: "power action failed"}
	}
}

// powerResultMatch is the reply rule for power.result: the envelope ID must
// equal the payload request ID and the result must come from the exact asset.
// Execute also requires the exact connection the action was sent on.
var powerResultMatch = agentmgr.Match{Key: agentmgr.KeyBoth, Device: agentmgr.DeviceExact}

// HandleResult validates and delivers an agent result that came in on conn.
// It returns false for malformed, stale, duplicate, cross-asset or
// other-connection results; those never unblock an HTTP request and
// therefore can never create false success.
func (c *Coordinator) HandleResult(conn *agentmgr.AgentConn, msg agentmgr.Message) bool {
	if c == nil || conn == nil || msg.Type != agentmgr.MsgPowerResult {
		return false
	}

	result, err := agentmgr.DecodeStrictly[agentmgr.PowerResultData](msg)
	if err != nil || result.Validate() != nil {
		return false
	}
	return c.pending.Deliver(powerResultMatch, agentmgr.Reply[agentmgr.PowerResultData]{
		EnvelopeID: msg.ID,
		PayloadID:  result.RequestID,
		Conn:       conn,
		Value:      result,
	})
}
