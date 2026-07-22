package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/powercontrol"
)

type powerTestSender struct {
	connected bool
	sendErr   error
	onSend    func(string, agentmgr.Message)
	sentCh    chan agentmgr.Message
	mu        sync.Mutex
	count     int
}

func (s *powerTestSender) IsConnected(string) bool { return s.connected }

func (s *powerTestSender) SendToAgent(assetID string, msg agentmgr.Message) error {
	s.mu.Lock()
	s.count++
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

func installPowerResponder(t *testing.T, sut *apiServer, status agentmgr.PowerResultStatus, code agentmgr.PowerResultCode) *powerTestSender {
	t.Helper()
	sender := &powerTestSender{connected: true}
	coordinator := powercontrol.New(sender, 50*time.Millisecond)
	sender.onSend = func(assetID string, msg agentmgr.Message) {
		var action agentmgr.PowerActionData
		if err := json.Unmarshal(msg.Data, &action); err != nil {
			t.Fatalf("decode outbound action: %v", err)
		}
		result := agentmgr.PowerResultData{
			RequestID: action.RequestID,
			AssetID:   action.AssetID,
			Action:    action.Action,
			Status:    status,
			Code:      code,
		}
		if status == agentmgr.PowerResultAccepted {
			result.Message = "operating system accepted power action"
		} else {
			result.Message = "power action was not accepted"
		}
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("encode result: %v", err)
		}
		if !coordinator.HandleResult(assetID, agentmgr.Message{Type: agentmgr.MsgPowerResult, ID: action.RequestID, Data: raw}) {
			t.Fatal("result was not delivered")
		}
	}
	sut.powerCoordinator = coordinator
	return sender
}

func powerRequest(method, path string, scopes, allowed []string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	ctx := contextWithPrincipal(req.Context(), "operator-1", "operator")
	ctx = contextWithScopes(ctx, scopes)
	ctx = contextWithAllowedAssets(ctx, allowed)
	return req.WithContext(ctx)
}

func TestV2PowerActionRequiresAgentAcceptanceAndAudits(t *testing.T) {
	sut := newTestAPIServer(t)
	installPowerResponder(t, sut, agentmgr.PowerResultAccepted, "")

	req := powerRequest(http.MethodPost, "/api/v2/assets/node-1/reboot", nil, nil)
	rec := httptest.NewRecorder()
	sut.handleV2AssetReboot(rec, req, "node-1")

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data["status"] != "rebooting" || body.Data["asset_id"] != "node-1" || !strings.HasPrefix(body.Data["request_id"], "power_") {
		t.Fatalf("unexpected body: %+v", body)
	}

	events, err := sut.auditStore.List(10, 0)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(events) != 1 || events[0].Type != "asset.power" || events[0].Decision != "accepted" || events[0].Target != "node-1" {
		t.Fatalf("unexpected audit events: %+v", events)
	}
	if events[0].Details["action"] != "reboot" || events[0].Details["agent_status"] != "accepted" {
		t.Fatalf("unexpected audit details: %+v", events[0].Details)
	}
}

func TestV2PowerActionMapsTypedFailuresToNon2xx(t *testing.T) {
	tests := []struct {
		name       string
		status     agentmgr.PowerResultStatus
		code       agentmgr.PowerResultCode
		wantStatus int
		wantCode   string
	}{
		{"unsupported", agentmgr.PowerResultUnsupported, agentmgr.PowerResultCodeUnsupportedPlatform, http.StatusUnprocessableEntity, "power_unsupported"},
		{"rejected", agentmgr.PowerResultRejected, agentmgr.PowerResultCodeCapabilityDenied, http.StatusConflict, "power_rejected"},
		{"failed", agentmgr.PowerResultFailed, agentmgr.PowerResultCodeExecutionFailed, http.StatusBadGateway, "power_execution_failed"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sut := newTestAPIServer(t)
			installPowerResponder(t, sut, tc.status, tc.code)
			rec := httptest.NewRecorder()
			sut.handleV2AssetShutdown(rec, powerRequest(http.MethodPost, "/api/v2/assets/node-1/shutdown", nil, nil), "node-1")
			if rec.Code != tc.wantStatus || !strings.Contains(rec.Body.String(), tc.wantCode) {
				t.Fatalf("status=%d body=%s, want %d/%s", rec.Code, rec.Body.String(), tc.wantStatus, tc.wantCode)
			}
		})
	}
}

func TestV2PowerActionOfflineDeliveryAndTimeoutNeverSucceed(t *testing.T) {
	tests := []struct {
		name       string
		sender     *powerTestSender
		timeout    time.Duration
		wantStatus int
		wantCode   string
	}{
		{"offline", &powerTestSender{}, time.Second, http.StatusConflict, "asset_offline"},
		{"send failed", &powerTestSender{connected: true, sendErr: errors.New("closed")}, time.Second, http.StatusBadGateway, "power_delivery_failed"},
		{"timeout", &powerTestSender{connected: true}, 10 * time.Millisecond, http.StatusGatewayTimeout, "power_timed_out"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sut := newTestAPIServer(t)
			sut.powerCoordinator = powercontrol.New(tc.sender, tc.timeout)
			rec := httptest.NewRecorder()
			sut.handleV2AssetReboot(rec, powerRequest(http.MethodPost, "/api/v2/assets/node-1/reboot", nil, nil), "node-1")
			if rec.Code != tc.wantStatus || !strings.Contains(rec.Body.String(), tc.wantCode) {
				t.Fatalf("status=%d body=%s, want %d/%s", rec.Code, rec.Body.String(), tc.wantStatus, tc.wantCode)
			}
		})
	}
}

func TestV2PowerActionReturnsTooManyRequestsAtGlobalAdmissionLimit(t *testing.T) {
	sut := newTestAPIServer(t)
	sender := &powerTestSender{connected: true, sentCh: make(chan agentmgr.Message, 1)}
	coordinator := powercontrol.NewWithLimit(sender, time.Second, 1)
	sut.powerCoordinator = coordinator

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

	rec := httptest.NewRecorder()
	sut.handleV2AssetShutdown(rec, powerRequest(http.MethodPost, "/api/v2/assets/node-2/shutdown", nil, nil), "node-2")
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "power_busy") {
		t.Fatalf("status=%d body=%s, want 429/power_busy", rec.Code, rec.Body.String())
	}

	cancelFirst()
	select {
	case err := <-firstDone:
		if powercontrol.KindOf(err) != powercontrol.ErrorCanceled {
			t.Fatalf("first action kind=%q err=%v", powercontrol.KindOf(err), err)
		}
	case <-time.After(time.Second):
		t.Fatal("first action did not finish after cancellation")
	}
}

func TestV2PowerActionEnforcesScopeAssetAllowlistAndRateLimit(t *testing.T) {
	t.Run("scope", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sender := installPowerResponder(t, sut, agentmgr.PowerResultAccepted, "")
		rec := httptest.NewRecorder()
		sut.handleV2AssetReboot(rec, powerRequest(http.MethodPost, "/api/v2/assets/node-1/reboot", []string{"assets:read"}, nil), "node-1")
		if rec.Code != http.StatusForbidden || sender.count != 0 {
			t.Fatalf("status=%d sends=%d body=%s", rec.Code, sender.count, rec.Body.String())
		}
	})

	t.Run("asset allowlist", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sender := installPowerResponder(t, sut, agentmgr.PowerResultAccepted, "")
		rec := httptest.NewRecorder()
		sut.handleV2AssetReboot(rec, powerRequest(http.MethodPost, "/api/v2/assets/node-1/reboot", []string{"assets:power"}, []string{"node-2"}), "node-1")
		if rec.Code != http.StatusForbidden || sender.count != 0 {
			t.Fatalf("status=%d sends=%d body=%s", rec.Code, sender.count, rec.Body.String())
		}
	})

	t.Run("rate limit", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sender := installPowerResponder(t, sut, agentmgr.PowerResultAccepted, "")
		for i := 0; i < powerActionRateLimit; i++ {
			rec := httptest.NewRecorder()
			sut.handleV2AssetReboot(rec, powerRequest(http.MethodPost, "/api/v2/assets/node-1/reboot", nil, nil), "node-1")
			if rec.Code != http.StatusAccepted {
				t.Fatalf("request %d status=%d body=%s", i, rec.Code, rec.Body.String())
			}
		}
		rec := httptest.NewRecorder()
		sut.handleV2AssetReboot(rec, powerRequest(http.MethodPost, "/api/v2/assets/node-1/reboot", nil, nil), "node-1")
		if rec.Code != http.StatusTooManyRequests || sender.count != powerActionRateLimit {
			t.Fatalf("status=%d sends=%d body=%s", rec.Code, sender.count, rec.Body.String())
		}
	})
}

func TestV2PowerActionMaintenanceBlockPreventsAgentDispatch(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.groupMaintenanceStore = &maintenanceOnlyGroupMaintenanceStore{}
	sender := installPowerResponder(t, sut, agentmgr.PowerResultAccepted, "")
	groupID := mustCreateGroup(t, sut, "Power Maintenance", "power-maintenance")
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "node-1",
		GroupID: groupID,
		Status:  "online",
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	if _, err := sut.groupMaintenanceStore.CreateGroupMaintenanceWindow(groupID, groupmaintenance.CreateMaintenanceWindowRequest{
		Name:         "Block power",
		StartAt:      time.Now().UTC().Add(-time.Minute),
		EndAt:        time.Now().UTC().Add(time.Minute),
		BlockActions: true,
	}); err != nil {
		t.Fatalf("create maintenance window: %v", err)
	}

	recorder := httptest.NewRecorder()
	sut.handleV2AssetReboot(recorder, powerRequest(http.MethodPost, "/api/v2/assets/node-1/reboot", nil, nil), "node-1")

	if recorder.Code != http.StatusLocked || !strings.Contains(recorder.Body.String(), "maintenance_blocked") {
		t.Fatalf("status=%d body=%s, want 423/maintenance_blocked", recorder.Code, recorder.Body.String())
	}
	if sender.count != 0 {
		t.Fatalf("agent sends = %d, want zero", sender.count)
	}
	events, err := sut.auditStore.List(10, 0)
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) != 1 || events[0].Type != "asset.power" || events[0].Decision != "denied" || events[0].Reason != "maintenance_blocked" {
		t.Fatalf("unexpected power audit events: %+v", events)
	}
}

func TestPowerResultIsRegisteredInWebSocketRouter(t *testing.T) {
	sut := newTestAPIServer(t)
	if _, ok := sut.buildWSRouter()[agentmgr.MsgPowerResult]; !ok {
		t.Fatal("power.result handler missing from websocket router")
	}
}
