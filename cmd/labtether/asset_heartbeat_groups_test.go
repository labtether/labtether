package main

import (
	"bytes"
	"encoding/json"
	"github.com/labtether/labtether/internal/assets"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAssetHeartbeatCreatesAsset(t *testing.T) {
	sut := newTestAPIServer(t)

	payload := []byte(`{"asset_id":"lab-host-01","type":"host","name":"Lab Host 01","source":"agent","status":"online","platform":"linux"}`)
	req := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	sut.handleAssetActions(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/assets", nil)
	listRec := httptest.NewRecorder()
	sut.handleAssets(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}

	var response struct {
		Assets []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode assets list: %v", err)
	}
	if len(response.Assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(response.Assets))
	}
	if response.Assets[0].ID != "lab-host-01" {
		t.Fatalf("expected asset ID lab-host-01, got %s", response.Assets[0].ID)
	}
}

func TestAssetHeartbeatNormalizesPlatform(t *testing.T) {
	sut := newTestAPIServer(t)

	payload := []byte(`{"asset_id":"lab-host-platform","type":"host","name":"Lab Host Platform","source":"agent","status":"online","platform":"Ubuntu 24.04 LTS","metadata":{"os_name":"Ubuntu 24.04 LTS"}}`)
	createReq := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(payload))
	createRec := httptest.NewRecorder()
	sut.handleAssetActions(createRec, createReq)

	if createRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", createRec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/assets/lab-host-platform", nil)
	getRec := httptest.NewRecorder()
	sut.handleAssetActions(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}

	var response struct {
		Asset struct {
			Platform string            `json:"platform"`
			Metadata map[string]string `json:"metadata"`
		} `json:"asset"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode asset response: %v", err)
	}
	if response.Asset.Platform != "linux" {
		t.Fatalf("expected canonical platform linux, got %s", response.Asset.Platform)
	}
	if response.Asset.Metadata["platform"] != "linux" {
		t.Fatalf("expected metadata platform linux, got %s", response.Asset.Metadata["platform"])
	}
}

func TestAssetHeartbeatRejectsUnknownGroup(t *testing.T) {
	sut := newTestAPIServer(t)

	payload := []byte(`{"asset_id":"lab-host-unknown-group","type":"host","name":"Lab Host Unknown Group","source":"agent","group_id":"group_missing","status":"online","platform":"linux"}`)
	req := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	sut.handleAssetActions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestUpdateAssetNamePersistsAcrossHeartbeats(t *testing.T) {
	sut := newTestAPIServer(t)

	initialHeartbeat := []byte(`{"asset_id":"lab-host-rename-persist","type":"host","name":"Initial Name","source":"agent","status":"online","platform":"linux","metadata":{"cpu_percent":"10.0"}}`)
	initialReq := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(initialHeartbeat))
	initialRec := httptest.NewRecorder()
	sut.handleAssetActions(initialRec, initialReq)
	if initialRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", initialRec.Code)
	}

	renameReq := httptest.NewRequest(http.MethodPatch, "/assets/lab-host-rename-persist", bytes.NewReader([]byte(`{"name":"Manual Name"}`)))
	renameRec := httptest.NewRecorder()
	sut.handleAssetActions(renameRec, renameReq)
	if renameRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", renameRec.Code)
	}

	var renameResp struct {
		Asset struct {
			Name     string            `json:"name"`
			Metadata map[string]string `json:"metadata"`
		} `json:"asset"`
	}
	if err := json.Unmarshal(renameRec.Body.Bytes(), &renameResp); err != nil {
		t.Fatalf("failed to decode rename response: %v", err)
	}
	if renameResp.Asset.Name != "Manual Name" {
		t.Fatalf("renamed name = %q, want %q", renameResp.Asset.Name, "Manual Name")
	}
	if renameResp.Asset.Metadata[assets.MetadataKeyNameOverride] != "Manual Name" {
		t.Fatalf("name override metadata = %q, want %q", renameResp.Asset.Metadata[assets.MetadataKeyNameOverride], "Manual Name")
	}

	secondHeartbeat := []byte(`{"asset_id":"lab-host-rename-persist","type":"host","name":"Heartbeat Name","source":"agent","status":"online","platform":"linux","metadata":{"cpu_percent":"42.0"}}`)
	secondReq := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(secondHeartbeat))
	secondRec := httptest.NewRecorder()
	sut.handleAssetActions(secondRec, secondReq)
	if secondRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", secondRec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/assets/lab-host-rename-persist", nil)
	getRec := httptest.NewRecorder()
	sut.handleAssetActions(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}

	var getResp struct {
		Asset struct {
			Name     string            `json:"name"`
			Metadata map[string]string `json:"metadata"`
		} `json:"asset"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("failed to decode get response: %v", err)
	}
	if getResp.Asset.Name != "Manual Name" {
		t.Fatalf("name after heartbeat = %q, want %q", getResp.Asset.Name, "Manual Name")
	}
	if getResp.Asset.Metadata[assets.MetadataKeyNameOverride] != "Manual Name" {
		t.Fatalf("name override metadata after heartbeat = %q, want %q", getResp.Asset.Metadata[assets.MetadataKeyNameOverride], "Manual Name")
	}
	if getResp.Asset.Metadata["cpu_percent"] != "42.0" {
		t.Fatalf("expected latest heartbeat metadata cpu_percent=42.0, got %q", getResp.Asset.Metadata["cpu_percent"])
	}
}

func TestManualGroupAssignmentPersistsAcrossHeartbeatsWithoutGroupID(t *testing.T) {
	sut := newTestAPIServer(t)

	groupID := mustCreateGroup(t, sut, "Persistent Group", "PERSISTENT-GROUP")

	initialHeartbeat := []byte(`{"asset_id":"lab-host-group-persist","type":"host","name":"Initial Name","source":"agent","status":"online","platform":"linux"}`)
	initialReq := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(initialHeartbeat))
	initialRec := httptest.NewRecorder()
	sut.handleAssetActions(initialRec, initialReq)
	if initialRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", initialRec.Code)
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/assets/lab-host-group-persist", bytes.NewReader([]byte(`{"group_id":"`+groupID+`"}`)))
	updateRec := httptest.NewRecorder()
	sut.handleAssetActions(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", updateRec.Code)
	}

	secondHeartbeat := []byte(`{"asset_id":"lab-host-group-persist","type":"host","name":"Heartbeat Name","source":"agent","status":"online","platform":"linux","metadata":{"cpu_percent":"42.0"}}`)
	secondReq := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(secondHeartbeat))
	secondRec := httptest.NewRecorder()
	sut.handleAssetActions(secondRec, secondReq)
	if secondRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", secondRec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/assets/lab-host-group-persist", nil)
	getRec := httptest.NewRecorder()
	sut.handleAssetActions(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}

	var getResp struct {
		Asset struct {
			GroupID  string            `json:"group_id"`
			Metadata map[string]string `json:"metadata"`
		} `json:"asset"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("failed to decode get response: %v", err)
	}
	if getResp.Asset.GroupID != groupID {
		t.Fatalf("group_id after heartbeat = %q, want %q", getResp.Asset.GroupID, groupID)
	}
	if getResp.Asset.Metadata["cpu_percent"] != "42.0" {
		t.Fatalf("expected latest heartbeat metadata cpu_percent=42.0, got %q", getResp.Asset.Metadata["cpu_percent"])
	}
}
