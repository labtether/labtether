package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
)

func TestDeadLettersEndpoint(t *testing.T) {
	sut := newTestAPIServer(t)
	logStore := sut.logStore.(*persistence.MemoryLogStore)

	if err := logStore.AppendEvent(logs.Event{
		ID:      "log_dead_letter_test_1",
		Source:  "dead_letter",
		Level:   "error",
		Message: "dead-letter from worker.command.decode",
		Fields: map[string]string{
			"event_id":   "dlq_1",
			"component":  "worker.command.decode",
			"subject":    "terminal.commands.requested",
			"deliveries": "5",
			"error":      "decode failed",
		},
	}); err != nil {
		t.Fatalf("failed to seed dead-letter log: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/queue/dead-letters?window=24h&limit=10", nil)
	rec := httptest.NewRecorder()
	sut.handleDeadLetters(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload struct {
		Events []struct {
			ID         string `json:"id"`
			Component  string `json:"component"`
			Subject    string `json:"subject"`
			Deliveries uint64 `json:"deliveries"`
		} `json:"events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode dead-letter response: %v", err)
	}
	if len(payload.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(payload.Events))
	}
	if payload.Events[0].ID != "dlq_1" {
		t.Fatalf("expected dead-letter ID dlq_1, got %s", payload.Events[0].ID)
	}
	if payload.Events[0].Deliveries != 5 {
		t.Fatalf("expected deliveries=5, got %d", payload.Events[0].Deliveries)
	}
}

func TestDeadLettersEndpointIncludesAnalytics(t *testing.T) {
	sut := newTestAPIServer(t)
	logStore := sut.logStore.(*persistence.MemoryLogStore)

	_ = logStore.AppendEvent(logs.Event{
		ID:      "log_dead_letter_analytics_1",
		Source:  "dead_letter",
		Level:   "error",
		Message: "network timeout",
		Fields: map[string]string{
			"event_id":   "dlq_analytics_1",
			"component":  "worker.command.result_publish",
			"subject":    "terminal.commands.completed",
			"deliveries": "3",
			"error":      "dial tcp timeout",
		},
		Timestamp: time.Now().UTC().Add(-5 * time.Minute),
	})
	_ = logStore.AppendEvent(logs.Event{
		ID:      "log_dead_letter_analytics_2",
		Source:  "dead_letter",
		Level:   "error",
		Message: "decode failure",
		Fields: map[string]string{
			"event_id":   "dlq_analytics_2",
			"component":  "worker.command.decode",
			"subject":    "terminal.commands.requested",
			"deliveries": "5",
			"error":      "json decode failed",
		},
		Timestamp: time.Now().UTC().Add(-3 * time.Minute),
	})

	req := httptest.NewRequest(http.MethodGet, "/queue/dead-letters?window=24h&limit=5", nil)
	rec := httptest.NewRecorder()
	sut.handleDeadLetters(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload struct {
		Total     int `json:"total"`
		Analytics struct {
			Total           int `json:"total"`
			TopErrorClasses []struct {
				Key   string `json:"key"`
				Count int    `json:"count"`
			} `json:"top_error_classes"`
		} `json:"analytics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode dead-letter analytics response: %v", err)
	}
	if payload.Total < 2 {
		t.Fatalf("expected total >= 2, got %d", payload.Total)
	}
	if payload.Analytics.Total < 2 {
		t.Fatalf("expected analytics total >= 2, got %d", payload.Analytics.Total)
	}
	if len(payload.Analytics.TopErrorClasses) == 0 {
		t.Fatalf("expected top_error_classes")
	}
}

func TestDeadLettersEndpointTotalUsesExactCountBeyondProjectionLimit(t *testing.T) {
	sut := newTestAPIServer(t)
	logStore := sut.logStore.(*persistence.MemoryLogStore)
	now := time.Now().UTC()

	const totalEvents = 1205
	for idx := 0; idx < totalEvents; idx++ {
		if err := logStore.AppendEvent(logs.Event{
			ID:      "log_dead_letter_total_" + strconv.Itoa(idx),
			Source:  "dead_letter",
			Level:   "error",
			Message: "timeout",
			Fields: map[string]string{
				"event_id":  "dlq_total_" + strconv.Itoa(idx),
				"component": "worker.command.result_publish",
				"subject":   "terminal.commands.completed",
			},
			Timestamp: now.Add(-time.Duration(idx%120) * time.Second),
		}); err != nil {
			t.Fatalf("failed to seed dead-letter log %d: %v", idx, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/queue/dead-letters?window=24h&limit=5", nil)
	rec := httptest.NewRecorder()
	sut.handleDeadLetters(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload struct {
		Listed    int        `json:"listed"`
		Total     int        `json:"total"`
		Events    []struct{} `json:"events"`
		Analytics struct {
			Total int `json:"total"`
		} `json:"analytics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode dead-letter response: %v", err)
	}
	if payload.Listed != 5 {
		t.Fatalf("expected listed=5, got %d", payload.Listed)
	}
	if len(payload.Events) != 5 {
		t.Fatalf("expected 5 listed events, got %d", len(payload.Events))
	}
	if payload.Total != totalEvents {
		t.Fatalf("expected total=%d, got %d", totalEvents, payload.Total)
	}
	if payload.Analytics.Total != totalEvents {
		t.Fatalf("expected analytics total=%d, got %d", totalEvents, payload.Analytics.Total)
	}
}

func TestHandleAuditEventsReturnsServiceUnavailableWithoutAuditStore(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.auditStore = nil

	req := httptest.NewRequest(http.MethodGet, "/audit/events?limit=5", nil)
	rec := httptest.NewRecorder()
	sut.handleAuditEvents(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "audit store unavailable") {
		t.Fatalf("expected audit store unavailable message, got %s", rec.Body.String())
	}
}

func TestCredentialProfilesCreateRotateAndAssetTerminalConfig(t *testing.T) {
	sut := newTestAPIServer(t)

	createPayload := []byte(`{"name":"Lab SSH","kind":"ssh_password","username":"root","secret":"super-secret"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/credentials/profiles", bytes.NewReader(createPayload))
	createRec := httptest.NewRecorder()
	sut.handleCredentialProfiles(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRec.Code)
	}

	var createResponse struct {
		Profile credentials.Profile `json:"profile"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResponse); err != nil {
		t.Fatalf("failed to decode profile create response: %v", err)
	}
	if createResponse.Profile.ID == "" {
		t.Fatalf("expected profile id")
	}
	if createResponse.Profile.SecretCiphertext != "" {
		t.Fatalf("secret ciphertext should not be exposed in response")
	}

	rotatePayload := []byte(`{"secret":"rotated-secret","reason":"test rotation"}`)
	rotateReq := httptest.NewRequest(http.MethodPost, "/credentials/profiles/"+createResponse.Profile.ID+"/rotate", bytes.NewReader(rotatePayload))
	rotateRec := httptest.NewRecorder()
	sut.handleCredentialProfileActions(rotateRec, rotateReq)
	if rotateRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rotateRec.Code)
	}

	heartbeatPayload := []byte(`{"asset_id":"ssh-host-1","type":"host","name":"SSH Host 1","source":"agent","status":"online","platform":"linux"}`)
	heartbeatReq := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(heartbeatPayload))
	heartbeatRec := httptest.NewRecorder()
	sut.handleAssetActions(heartbeatRec, heartbeatReq)
	if heartbeatRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", heartbeatRec.Code)
	}

	configPayload := []byte(`{"host":"10.0.0.12","port":22,"username":"root","strict_host_key":true,"host_key":"SHA256:fake-fingerprint","credential_profile_id":"` + createResponse.Profile.ID + `"}`)
	configReq := httptest.NewRequest(http.MethodPut, "/assets/ssh-host-1/terminal/config", bytes.NewReader(configPayload))
	configRec := httptest.NewRecorder()
	sut.handleAssetActions(configRec, configReq)
	if configRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", configRec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/assets/ssh-host-1/terminal/config", nil)
	getRec := httptest.NewRecorder()
	sut.handleAssetActions(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}

	var getResponse struct {
		TerminalConfig struct {
			AssetID             string `json:"asset_id"`
			Host                string `json:"host"`
			CredentialProfileID string `json:"credential_profile_id"`
		} `json:"terminal_config"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResponse); err != nil {
		t.Fatalf("failed to decode terminal config response: %v", err)
	}
	if getResponse.TerminalConfig.AssetID != "ssh-host-1" {
		t.Fatalf("expected asset_id ssh-host-1, got %s", getResponse.TerminalConfig.AssetID)
	}
	if getResponse.TerminalConfig.CredentialProfileID != createResponse.Profile.ID {
		t.Fatalf("expected credential profile id %s, got %s", createResponse.Profile.ID, getResponse.TerminalConfig.CredentialProfileID)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/assets/ssh-host-1/terminal/config", nil)
	deleteRec := httptest.NewRecorder()
	sut.handleAssetActions(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", deleteRec.Code)
	}

	getAfterDeleteReq := httptest.NewRequest(http.MethodGet, "/assets/ssh-host-1/terminal/config", nil)
	getAfterDeleteRec := httptest.NewRecorder()
	sut.handleAssetActions(getAfterDeleteRec, getAfterDeleteReq)
	if getAfterDeleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200 after delete, got %d", getAfterDeleteRec.Code)
	}

	var getAfterDeleteResponse struct {
		TerminalConfig any `json:"terminal_config"`
	}
	if err := json.Unmarshal(getAfterDeleteRec.Body.Bytes(), &getAfterDeleteResponse); err != nil {
		t.Fatalf("failed to decode terminal config after delete response: %v", err)
	}
	if getAfterDeleteResponse.TerminalConfig != nil {
		t.Fatalf("expected nil terminal_config after delete, got %#v", getAfterDeleteResponse.TerminalConfig)
	}
}

func TestCredentialProfilesCreateAcceptsProxmoxPasswordKind(t *testing.T) {
	sut := newTestAPIServer(t)

	createPayload := []byte(`{"name":"Proxmox Password","kind":"proxmox_password","username":"root@pam","secret":"super-secret"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/credentials/profiles", bytes.NewReader(createPayload))
	createRec := httptest.NewRecorder()
	sut.handleCredentialProfiles(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d with body %s", createRec.Code, createRec.Body.String())
	}

	var createResponse struct {
		Profile credentials.Profile `json:"profile"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResponse); err != nil {
		t.Fatalf("failed to decode profile create response: %v", err)
	}
	if createResponse.Profile.Kind != credentials.KindProxmoxPassword {
		t.Fatalf("expected kind %q, got %q", credentials.KindProxmoxPassword, createResponse.Profile.Kind)
	}
	if createResponse.Profile.Username != "root@pam" {
		t.Fatalf("expected username root@pam, got %q", createResponse.Profile.Username)
	}
}

func TestDesktopCredentialEndpointsRequireManagedAsset(t *testing.T) {
	sut := newTestAPIServer(t)

	getReq := httptest.NewRequest(http.MethodGet, "/assets/missing-node/desktop/credentials", nil)
	getRec := httptest.NewRecorder()
	sut.handleDesktopCredentials(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unmanaged asset credentials lookup, got %d", getRec.Code)
	}

	retrieveReq := httptest.NewRequest(http.MethodPost, "/assets/missing-node/desktop/credentials/retrieve", nil)
	retrieveRec := httptest.NewRecorder()
	sut.handleRetrieveDesktopCredentials(retrieveRec, retrieveReq)
	if retrieveRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unmanaged asset credential retrieval, got %d", retrieveRec.Code)
	}
}
