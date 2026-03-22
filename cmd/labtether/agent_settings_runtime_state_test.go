package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentcore"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
)

func TestNormalizeAgentSettingValuesRejectsLocalOnlyForHubApply(t *testing.T) {
	if _, err := normalizeAgentSettingValues(map[string]string{
		agentcore.SettingKeyTLSSkipVerify: "true",
	}, true); err == nil {
		t.Fatalf("expected local-only setting to be rejected for hub apply")
	}

	values, err := normalizeAgentSettingValues(map[string]string{
		agentcore.SettingKeyTLSSkipVerify: "true",
	}, false)
	if err != nil {
		t.Fatalf("expected local-only setting to normalize for local apply, got %v", err)
	}
	if values[agentcore.SettingKeyTLSSkipVerify] != "true" {
		t.Fatalf("normalized value=%q, want true", values[agentcore.SettingKeyTLSSkipVerify])
	}
}

func TestPushAgentSettingsApplyIncludesStoredFingerprint(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "node-apply",
		Type:    "device",
		Name:    "node-apply",
		Source:  "agent",
		Metadata: map[string]string{
			"agent_device_fingerprint": "fp-expected",
		},
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-apply", "linux"))
	defer sut.agentMgr.Unregister("node-apply")

	done := make(chan agentmgr.Message, 1)
	go func() {
		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound settings apply: %v", err)
			return
		}
		done <- outbound
	}()

	sut.pushAgentSettingsApply("node-apply", map[string]string{
		agentcore.SettingKeyLogLevel: "debug",
	})

	select {
	case outbound := <-done:
		if outbound.Type != agentmgr.MsgAgentSettingsApply {
			t.Fatalf("message type=%q, want %q", outbound.Type, agentmgr.MsgAgentSettingsApply)
		}

		var payload agentmgr.AgentSettingsApplyData
		if err := json.Unmarshal(outbound.Data, &payload); err != nil {
			t.Fatalf("decode outbound payload: %v", err)
		}
		if payload.ExpectedFingerprint != "fp-expected" {
			t.Fatalf("expected fingerprint=%q, want fp-expected", payload.ExpectedFingerprint)
		}
		if payload.RequestID == "" || payload.Revision == "" {
			t.Fatalf("expected request and revision IDs, got %+v", payload)
		}
		if payload.Values[agentcore.SettingKeyLogLevel] != "debug" {
			t.Fatalf("log level=%q, want debug", payload.Values[agentcore.SettingKeyLogLevel])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for outbound settings apply")
	}

	state, ok := sut.getAgentSettingsRuntimeState("node-apply")
	if !ok {
		t.Fatalf("expected pending runtime state")
	}
	if state.Status != "pending" {
		t.Fatalf("status=%q, want pending", state.Status)
	}
	if state.Fingerprint != "fp-expected" {
		t.Fatalf("fingerprint=%q, want fp-expected", state.Fingerprint)
	}
	if state.Values[agentcore.SettingKeyLogLevel] != "debug" {
		t.Fatalf("state log level=%q, want debug", state.Values[agentcore.SettingKeyLogLevel])
	}
}

func TestProcessAgentSettingsStatePreservesApplyLifecycleForMatchingRevision(t *testing.T) {
	sut := newTestAPIServer(t)
	assetID := "node-lifecycle"
	appliedAt := time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC)
	reportedAt := time.Date(2026, 3, 8, 10, 1, 0, 0, time.UTC)

	tests := []struct {
		name                string
		previous            agentSettingsRuntimeState
		wantStatus          string
		wantLastError       string
		wantRestartRequired bool
	}{
		{
			name: "applied result survives state report",
			previous: agentSettingsRuntimeState{
				Status:          "applied",
				Revision:        "rev-1",
				AppliedAt:       appliedAt,
				RestartRequired: true,
				Fingerprint:     "fp-before",
			},
			wantStatus:          "applied",
			wantRestartRequired: true,
		},
		{
			name: "failed result survives state report",
			previous: agentSettingsRuntimeState{
				Status:          "failed",
				Revision:        "rev-1",
				AppliedAt:       appliedAt,
				LastError:       "fingerprint mismatch",
				RestartRequired: false,
				Fingerprint:     "fp-before",
			},
			wantStatus:    "failed",
			wantLastError: "fingerprint mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sut.setAgentSettingsRuntimeState(assetID, tt.previous)

			payload, err := json.Marshal(agentmgr.AgentSettingsStateData{
				Revision:             "rev-1",
				Values:               map[string]string{agentcore.SettingKeyDockerEnabled: "true"},
				Fingerprint:          "fp-after",
				AllowRemoteOverrides: true,
				ReportedAt:           reportedAt.Format(time.RFC3339),
			})
			if err != nil {
				t.Fatalf("marshal state payload: %v", err)
			}

			sut.processAgentSettingsState(&agentmgr.AgentConn{AssetID: assetID}, agentmgr.Message{Data: payload})

			state, ok := sut.getAgentSettingsRuntimeState(assetID)
			if !ok {
				t.Fatalf("expected runtime state")
			}
			if state.Status != tt.wantStatus {
				t.Fatalf("status=%q, want %q", state.Status, tt.wantStatus)
			}
			if state.LastError != tt.wantLastError {
				t.Fatalf("last_error=%q, want %q", state.LastError, tt.wantLastError)
			}
			if !state.AppliedAt.Equal(appliedAt) {
				t.Fatalf("applied_at=%v, want %v", state.AppliedAt, appliedAt)
			}
			if state.RestartRequired != tt.wantRestartRequired {
				t.Fatalf("restart_required=%v, want %v", state.RestartRequired, tt.wantRestartRequired)
			}
			if !state.UpdatedAt.Equal(reportedAt) {
				t.Fatalf("updated_at=%v, want %v", state.UpdatedAt, reportedAt)
			}
			if state.Fingerprint != "fp-after" {
				t.Fatalf("fingerprint=%q, want fp-after", state.Fingerprint)
			}
			if !state.AllowRemoteOverrides {
				t.Fatalf("expected allow_remote_overrides=true")
			}
			if state.Values[agentcore.SettingKeyDockerEnabled] != "true" {
				t.Fatalf("docker_enabled=%q, want true", state.Values[agentcore.SettingKeyDockerEnabled])
			}
		})
	}
}

func TestBuildAgentSettingsPayloadMarksDriftWhenAgentReportsEmptyValue(t *testing.T) {
	sut := newTestAPIServer(t)
	assetID := "node-drift"
	wantTURNURL := "turn:turn.example.com:3478"

	if _, err := sut.runtimeStore.SaveRuntimeSettingOverrides(map[string]string{
		agentSettingStoreKey(assetID, agentcore.SettingKeyWebRTCTURNURL): wantTURNURL,
	}); err != nil {
		t.Fatalf("save runtime override: %v", err)
	}

	sut.setAgentSettingsRuntimeState(assetID, agentSettingsRuntimeState{
		Status: "reported",
		Values: map[string]string{
			agentcore.SettingKeyWebRTCTURNURL: "",
		},
	})

	payload, err := sut.buildAgentSettingsPayload(assetID)
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}

	var found agentSettingEntry
	for _, setting := range payload.Settings {
		if setting.Key == agentcore.SettingKeyWebRTCTURNURL {
			found = setting
			break
		}
	}
	if found.Key == "" {
		t.Fatalf("expected %s setting in payload", agentcore.SettingKeyWebRTCTURNURL)
	}
	if found.EffectiveValue != wantTURNURL {
		t.Fatalf("effective_value=%q, want %q", found.EffectiveValue, wantTURNURL)
	}
	if !found.Drift {
		t.Fatalf("expected drift=true when reported empty value differs from desired value")
	}
	if found.Source != "hub-override" {
		t.Fatalf("source=%q, want hub-override", found.Source)
	}
}

func TestHandleAgentSettingsHistoryPreservesApplyFailureAfterStateReport(t *testing.T) {
	sut := newTestAPIServer(t)
	appliedAt := time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC)
	reportedAt := time.Date(2026, 3, 8, 10, 1, 0, 0, time.UTC)

	sut.setAgentSettingsRuntimeState("node-history", agentSettingsRuntimeState{
		Status:    "failed",
		Revision:  "rev-2",
		LastError: "fingerprint mismatch",
		AppliedAt: appliedAt,
	})

	payload, err := json.Marshal(agentmgr.AgentSettingsStateData{
		Revision:   "rev-2",
		Values:     map[string]string{agentcore.SettingKeyLogLevel: "info"},
		ReportedAt: reportedAt.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("marshal state payload: %v", err)
	}
	sut.processAgentSettingsState(&agentmgr.AgentConn{AssetID: "node-history"}, agentmgr.Message{Data: payload})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/node-history/settings/history", nil)
	rec := httptest.NewRecorder()
	sut.handleAgentSettingsHistory(rec, req, "node-history")

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"status":"failed"`) {
		t.Fatalf("expected failed status in history payload, got %s", body)
	}
	if !strings.Contains(body, `"last_error":"fingerprint mismatch"`) {
		t.Fatalf("expected last_error in history payload, got %s", body)
	}
	if !strings.Contains(body, `"applied_at":"2026-03-08T10:00:00Z"`) {
		t.Fatalf("expected applied_at in history payload, got %s", body)
	}
}
