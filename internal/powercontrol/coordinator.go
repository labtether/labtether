package powercontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
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
type Sender interface {
	IsConnected(assetID string) bool
	SendToAgent(assetID string, msg agentmgr.Message) error
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

type pendingAction struct {
	requestID string
	assetID   string
	action    agentmgr.PowerAction
	resultCh  chan agentmgr.PowerResultData
	delivered atomic.Bool
}

type Coordinator struct {
	sender    Sender
	timeout   time.Duration
	admission chan struct{}
	pending   sync.Map // request ID -> *pendingAction; bounded by admission
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
		sender:    sender,
		timeout:   timeout,
		admission: make(chan struct{}, maxConcurrent),
	}
}

// Execute sends one typed action to exactly one connected agent and waits for
// a strictly correlated power.result. Only an accepted result returns nil.
func (c *Coordinator) Execute(ctx context.Context, assetID string, action agentmgr.PowerAction) (agentmgr.PowerResultData, error) {
	assetID = strings.TrimSpace(assetID)
	if assetID == "" || len(assetID) > 256 || !action.Valid() {
		return agentmgr.PowerResultData{}, &ActionError{Kind: ErrorInvalid, Message: "invalid power action request"}
	}
	if c == nil || c.sender == nil || !c.sender.IsConnected(assetID) {
		return agentmgr.PowerResultData{}, &ActionError{Kind: ErrorAgentOffline, Message: "asset agent is not connected"}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case c.admission <- struct{}{}:
		defer func() { <-c.admission }()
	default:
		return agentmgr.PowerResultData{AssetID: assetID, Action: action}, &ActionError{
			Kind:    ErrorBusy,
			Message: "too many power actions are already in progress",
		}
	}

	requestID := idgen.New("power")
	pending := &pendingAction{
		requestID: requestID,
		assetID:   assetID,
		action:    action,
		resultCh:  make(chan agentmgr.PowerResultData, 1),
	}
	c.pending.Store(requestID, pending)
	defer c.pending.Delete(requestID)

	payload := agentmgr.PowerActionData{
		RequestID: requestID,
		AssetID:   assetID,
		Action:    action,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return agentmgr.PowerResultData{}, &ActionError{Kind: ErrorInvalid, RequestID: requestID, Message: "failed to encode power action"}
	}
	if err := c.sender.SendToAgent(assetID, agentmgr.Message{
		Type: agentmgr.MsgPowerAction,
		ID:   requestID,
		Data: data,
	}); err != nil {
		return agentmgr.PowerResultData{}, &ActionError{Kind: ErrorSendFailed, RequestID: requestID, Message: "failed to deliver power action to agent"}
	}

	timer := time.NewTimer(c.timeout)
	defer timer.Stop()
	select {
	case result := <-pending.resultCh:
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
	case <-ctx.Done():
		kind := ErrorCanceled
		message := "power action request canceled"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			kind = ErrorTimedOut
			message = "timed out waiting for the agent power result"
		}
		return agentmgr.PowerResultData{RequestID: requestID, AssetID: assetID, Action: action}, &ActionError{
			Kind:      kind,
			RequestID: requestID,
			Message:   message,
		}
	case <-timer.C:
		return agentmgr.PowerResultData{RequestID: requestID, AssetID: assetID, Action: action}, &ActionError{
			Kind:      ErrorTimedOut,
			RequestID: requestID,
			Message:   "timed out waiting for the agent power result",
		}
	}
}

// HandleResult validates and delivers an agent result. It returns false for
// malformed, stale, duplicate, or cross-asset results; those never unblock an
// HTTP request and therefore can never create false success.
func (c *Coordinator) HandleResult(connAssetID string, msg agentmgr.Message) bool {
	if c == nil || msg.Type != agentmgr.MsgPowerResult {
		return false
	}

	var result agentmgr.PowerResultData
	if err := decodeStrict(msg.Data, &result); err != nil || result.Validate() != nil {
		return false
	}
	if msg.ID == "" || msg.ID != result.RequestID {
		return false
	}

	raw, ok := c.pending.Load(result.RequestID)
	if !ok {
		return false
	}
	pending, ok := raw.(*pendingAction)
	if !ok || pending == nil {
		return false
	}
	if connAssetID != pending.assetID || result.AssetID != pending.assetID || result.Action != pending.action || result.RequestID != pending.requestID {
		return false
	}
	if !pending.delivered.CompareAndSwap(false, true) {
		return false
	}
	pending.resultCh <- result
	return true
}

func decodeStrict(raw []byte, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("power payload must contain exactly one object")
	}
	return nil
}
