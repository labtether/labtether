package powercontrol

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

type fakeSender struct {
	connected bool
	sendErr   error
	onSend    func(assetID string, msg agentmgr.Message)
	sentCh    chan agentmgr.Message
	mu        sync.Mutex
	sent      []agentmgr.Message
}

func (s *fakeSender) IsConnected(string) bool { return s.connected }

func (s *fakeSender) SendToAgent(assetID string, msg agentmgr.Message) error {
	s.mu.Lock()
	s.sent = append(s.sent, msg)
	s.mu.Unlock()
	if s.sentCh != nil {
		select {
		case s.sentCh <- msg:
		default:
		}
	}
	if s.sendErr != nil {
		return s.sendErr
	}
	if s.onSend != nil {
		s.onSend(assetID, msg)
	}
	return nil
}

func TestCoordinatorAcceptsStrictlyCorrelatedResult(t *testing.T) {
	sender := &fakeSender{connected: true}
	coordinator := New(sender, time.Second)
	sender.onSend = func(assetID string, msg agentmgr.Message) {
		var action agentmgr.PowerActionData
		if err := json.Unmarshal(msg.Data, &action); err != nil {
			t.Fatalf("decode action: %v", err)
		}
		if msg.Type != agentmgr.MsgPowerAction || msg.ID != action.RequestID || assetID != action.AssetID {
			t.Fatalf("uncorrelated outbound action: asset=%q msg=%+v action=%+v", assetID, msg, action)
		}
		result := agentmgr.PowerResultData{
			RequestID: action.RequestID,
			AssetID:   action.AssetID,
			Action:    action.Action,
			Status:    agentmgr.PowerResultAccepted,
			Message:   "operating system accepted reboot",
		}
		raw, _ := json.Marshal(result)
		if !coordinator.HandleResult(assetID, agentmgr.Message{Type: agentmgr.MsgPowerResult, ID: action.RequestID, Data: raw}) {
			t.Fatal("expected result delivery")
		}
	}

	result, err := coordinator.Execute(context.Background(), "node-1", agentmgr.PowerActionReboot)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Status != agentmgr.PowerResultAccepted || result.AssetID != "node-1" || result.Action != agentmgr.PowerActionReboot {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestCoordinatorRejectsCrossAssetAndCrossRequestResults(t *testing.T) {
	tests := []struct {
		name       string
		connAsset  string
		resultEdit func(*agentmgr.PowerResultData)
		messageID  func(agentmgr.PowerResultData) string
	}{
		{name: "wrong connection", connAsset: "node-2"},
		{name: "wrong payload asset", connAsset: "node-1", resultEdit: func(result *agentmgr.PowerResultData) { result.AssetID = "node-2" }},
		{name: "wrong action", connAsset: "node-1", resultEdit: func(result *agentmgr.PowerResultData) { result.Action = agentmgr.PowerActionShutdown }},
		{name: "wrong envelope id", connAsset: "node-1", messageID: func(agentmgr.PowerResultData) string { return "power-wrong" }},
		{name: "unknown field", connAsset: "node-1", resultEdit: func(result *agentmgr.PowerResultData) { result.Message = "__unknown_field__" }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sender := &fakeSender{connected: true}
			coordinator := New(sender, 20*time.Millisecond)
			sender.onSend = func(_ string, msg agentmgr.Message) {
				var action agentmgr.PowerActionData
				_ = json.Unmarshal(msg.Data, &action)
				result := agentmgr.PowerResultData{
					RequestID: action.RequestID,
					AssetID:   action.AssetID,
					Action:    action.Action,
					Status:    agentmgr.PowerResultAccepted,
				}
				if tc.resultEdit != nil {
					tc.resultEdit(&result)
				}
				messageID := result.RequestID
				if tc.messageID != nil {
					messageID = tc.messageID(result)
				}
				var raw []byte
				if result.Message == "__unknown_field__" {
					raw = []byte(`{"request_id":"` + result.RequestID + `","asset_id":"node-1","action":"reboot","status":"accepted","extra":true}`)
				} else {
					raw, _ = json.Marshal(result)
				}
				if coordinator.HandleResult(tc.connAsset, agentmgr.Message{Type: agentmgr.MsgPowerResult, ID: messageID, Data: raw}) {
					t.Fatal("mismatched result was delivered")
				}
			}

			_, err := coordinator.Execute(context.Background(), "node-1", agentmgr.PowerActionReboot)
			if KindOf(err) != ErrorTimedOut {
				t.Fatalf("expected timeout after ignored result, got %v", err)
			}
		})
	}
}

func TestCoordinatorMapsAgentFailures(t *testing.T) {
	tests := []struct {
		status agentmgr.PowerResultStatus
		code   agentmgr.PowerResultCode
		kind   ErrorKind
	}{
		{agentmgr.PowerResultUnsupported, agentmgr.PowerResultCodeUnsupportedPlatform, ErrorUnsupported},
		{agentmgr.PowerResultRejected, agentmgr.PowerResultCodeCapabilityDenied, ErrorRejected},
		{agentmgr.PowerResultFailed, agentmgr.PowerResultCodeExecutionFailed, ErrorExecutionFail},
	}
	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			sender := &fakeSender{connected: true}
			coordinator := New(sender, time.Second)
			sender.onSend = func(assetID string, msg agentmgr.Message) {
				var action agentmgr.PowerActionData
				_ = json.Unmarshal(msg.Data, &action)
				result := agentmgr.PowerResultData{
					RequestID: action.RequestID,
					AssetID:   action.AssetID,
					Action:    action.Action,
					Status:    tc.status,
					Code:      tc.code,
					Message:   "safe failure",
				}
				raw, _ := json.Marshal(result)
				coordinator.HandleResult(assetID, agentmgr.Message{Type: agentmgr.MsgPowerResult, ID: action.RequestID, Data: raw})
			}

			_, err := coordinator.Execute(context.Background(), "node-1", agentmgr.PowerActionReboot)
			if KindOf(err) != tc.kind {
				t.Fatalf("got kind %q err=%v, want %q", KindOf(err), err, tc.kind)
			}
		})
	}
}

func TestCoordinatorOfflineSendAndCancellation(t *testing.T) {
	coordinator := New(&fakeSender{}, time.Second)
	if _, err := coordinator.Execute(context.Background(), "node-1", agentmgr.PowerActionReboot); KindOf(err) != ErrorAgentOffline {
		t.Fatalf("offline kind=%q err=%v", KindOf(err), err)
	}

	coordinator = New(&fakeSender{connected: true, sendErr: errors.New("closed")}, time.Second)
	if _, err := coordinator.Execute(context.Background(), "node-1", agentmgr.PowerActionReboot); KindOf(err) != ErrorSendFailed {
		t.Fatalf("send kind=%q err=%v", KindOf(err), err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	coordinator = New(&fakeSender{connected: true}, time.Second)
	if _, err := coordinator.Execute(ctx, "node-1", agentmgr.PowerActionShutdown); KindOf(err) != ErrorCanceled {
		t.Fatalf("cancel kind=%q err=%v", KindOf(err), err)
	}
}

func TestCoordinatorGloballyBoundsPendingActions(t *testing.T) {
	sender := &fakeSender{connected: true, sentCh: make(chan agentmgr.Message, 1)}
	coordinator := NewWithLimit(sender, time.Second, 1)
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()

	firstDone := make(chan error, 1)
	go func() {
		_, err := coordinator.Execute(firstCtx, "node-1", agentmgr.PowerActionReboot)
		firstDone <- err
	}()

	select {
	case <-sender.sentCh:
	case <-time.After(time.Second):
		t.Fatal("first action was not sent")
	}

	if _, err := coordinator.Execute(context.Background(), "node-2", agentmgr.PowerActionShutdown); KindOf(err) != ErrorBusy {
		t.Fatalf("second action kind=%q err=%v, want %q", KindOf(err), err, ErrorBusy)
	}
	sender.mu.Lock()
	sendCount := len(sender.sent)
	sender.mu.Unlock()
	if sendCount != 1 {
		t.Fatalf("sent %d actions, want only the admitted action", sendCount)
	}

	cancelFirst()
	select {
	case err := <-firstDone:
		if KindOf(err) != ErrorCanceled {
			t.Fatalf("first action kind=%q err=%v, want %q", KindOf(err), err, ErrorCanceled)
		}
	case <-time.After(time.Second):
		t.Fatal("first action did not release admission after cancellation")
	}
}
