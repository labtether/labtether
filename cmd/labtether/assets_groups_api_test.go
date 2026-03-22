package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/assets"
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

func TestAssetResponseIncludesCanonicalFields(t *testing.T) {
	sut := newTestAPIServer(t)

	payload := []byte(`{"asset_id":"docker-ct-agent-01-abc123","type":"docker-container","name":"nginx","source":"docker","status":"online","metadata":{"cpu_percent":"12.5","memory_percent":"41.0","container_id":"abc123"}}`)
	createReq := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(payload))
	createRec := httptest.NewRecorder()
	sut.handleAssetActions(createRec, createReq)

	if createRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", createRec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/assets/docker-ct-agent-01-abc123", nil)
	getRec := httptest.NewRecorder()
	sut.handleAssetActions(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}

	var response struct {
		Asset struct {
			ResourceClass string         `json:"resource_class"`
			ResourceKind  string         `json:"resource_kind"`
			Attributes    map[string]any `json:"attributes"`
		} `json:"asset"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode asset response: %v", err)
	}
	if response.Asset.ResourceClass != "compute" {
		t.Fatalf("expected resource_class compute, got %s", response.Asset.ResourceClass)
	}
	if response.Asset.ResourceKind != "docker-container" {
		t.Fatalf("expected resource_kind docker-container, got %s", response.Asset.ResourceKind)
	}
	if response.Asset.Attributes == nil {
		t.Fatalf("expected attributes in response")
	}
	if got := response.Asset.Attributes["container_id"]; got != "abc123" {
		t.Fatalf("expected container_id abc123, got %#v", got)
	}
	if got, ok := response.Asset.Attributes["cpu_used_percent"].(float64); !ok || got != 12.5 {
		t.Fatalf("expected cpu_used_percent 12.5, got %#v", response.Asset.Attributes["cpu_used_percent"])
	}
}

func TestGetAssetByID(t *testing.T) {
	sut := newTestAPIServer(t)

	payload := []byte(`{"asset_id":"ha-main","type":"home-assistant","name":"HA Main","source":"homeassistant","status":"online","platform":"linux"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(payload))
	createRec := httptest.NewRecorder()
	sut.handleAssetActions(createRec, createReq)

	if createRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", createRec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/assets/ha-main", nil)
	getRec := httptest.NewRecorder()
	sut.handleAssetActions(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}
}

func TestGroupsCreateListAndAssignAsset(t *testing.T) {
	sut := newTestAPIServer(t)

	groupID := mustCreateGroup(t, sut, "Garage Lab", "garage")

	listReq := httptest.NewRequest(http.MethodGet, "/groups", nil)
	listRec := httptest.NewRecorder()
	sut.handleGroups(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}

	assetPayload := []byte(`{"asset_id":"lab-host-group","type":"host","name":"Lab Host Group","source":"agent","group_id":"` + groupID + `","status":"online","platform":"linux"}`)
	assetReq := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(assetPayload))
	assetRec := httptest.NewRecorder()
	sut.handleAssetActions(assetRec, assetReq)
	if assetRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", assetRec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/assets/lab-host-group", nil)
	getRec := httptest.NewRecorder()
	sut.handleAssetActions(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}

	var getResp struct {
		Asset struct {
			GroupID string `json:"group_id"`
		} `json:"asset"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("failed to decode asset response: %v", err)
	}
	if getResp.Asset.GroupID != groupID {
		t.Fatalf("expected group_id %s, got %s", groupID, getResp.Asset.GroupID)
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

func TestUpdateAssetNameAndGroup(t *testing.T) {
	sut := newTestAPIServer(t)

	createGroup := func(name, slug string) string {
		return mustCreateGroup(t, sut, name, slug)
	}

	groupOneID := createGroup("Garage Lab", "GARAGE-UPDATE")
	groupTwoID := createGroup("Office Lab", "OFFICE-UPDATE")

	assetPayload := []byte(`{"asset_id":"lab-host-update","type":"host","name":"Old Name","source":"agent","group_id":"` + groupOneID + `","status":"online","platform":"linux"}`)
	assetReq := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(assetPayload))
	assetRec := httptest.NewRecorder()
	sut.handleAssetActions(assetRec, assetReq)
	if assetRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", assetRec.Code)
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/assets/lab-host-update", bytes.NewReader([]byte(`{"name":"Renamed Host","group_id":"`+groupTwoID+`","tags":["Prod"," edge ","prod"]}`)))
	updateRec := httptest.NewRecorder()
	sut.handleAssetActions(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", updateRec.Code)
	}

	var updateResp struct {
		Asset struct {
			Name    string   `json:"name"`
			GroupID string   `json:"group_id"`
			Tags    []string `json:"tags"`
		} `json:"asset"`
	}
	if err := json.Unmarshal(updateRec.Body.Bytes(), &updateResp); err != nil {
		t.Fatalf("failed to decode update response: %v", err)
	}
	if updateResp.Asset.Name != "Renamed Host" {
		t.Fatalf("expected renamed asset, got %q", updateResp.Asset.Name)
	}
	if updateResp.Asset.GroupID != groupTwoID {
		t.Fatalf("expected group_id %q, got %q", groupTwoID, updateResp.Asset.GroupID)
	}
	if len(updateResp.Asset.Tags) != 2 || updateResp.Asset.Tags[0] != "edge" || updateResp.Asset.Tags[1] != "prod" {
		t.Fatalf("expected normalized tags [edge prod], got %v", updateResp.Asset.Tags)
	}

	clearReq := httptest.NewRequest(http.MethodPatch, "/assets/lab-host-update", bytes.NewReader([]byte(`{"group_id":""}`)))
	clearRec := httptest.NewRecorder()
	sut.handleAssetActions(clearRec, clearReq)
	if clearRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", clearRec.Code)
	}

	var clearResp struct {
		Asset struct {
			GroupID string   `json:"group_id"`
			Tags    []string `json:"tags"`
		} `json:"asset"`
	}
	if err := json.Unmarshal(clearRec.Body.Bytes(), &clearResp); err != nil {
		t.Fatalf("failed to decode clear response: %v", err)
	}
	if clearResp.Asset.GroupID != "" {
		t.Fatalf("expected empty group_id after clear, got %q", clearResp.Asset.GroupID)
	}
	if len(clearResp.Asset.Tags) != 2 || clearResp.Asset.Tags[0] != "edge" || clearResp.Asset.Tags[1] != "prod" {
		t.Fatalf("expected tags to be preserved after group clear, got %v", clearResp.Asset.Tags)
	}

	clearTagsReq := httptest.NewRequest(http.MethodPatch, "/assets/lab-host-update", bytes.NewReader([]byte(`{"tags":[]}`)))
	clearTagsRec := httptest.NewRecorder()
	sut.handleAssetActions(clearTagsRec, clearTagsReq)
	if clearTagsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", clearTagsRec.Code)
	}

	var clearTagsResp struct {
		Asset struct {
			Tags []string `json:"tags"`
		} `json:"asset"`
	}
	if err := json.Unmarshal(clearTagsRec.Body.Bytes(), &clearTagsResp); err != nil {
		t.Fatalf("failed to decode clear tags response: %v", err)
	}
	if len(clearTagsResp.Asset.Tags) != 0 {
		t.Fatalf("expected tags to be cleared, got %v", clearTagsResp.Asset.Tags)
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

func TestUpdateAssetGroupCascadesToPortainerSubDevices(t *testing.T) {
	sut := newTestAPIServer(t)

	createGroup := func(name, slug string) string {
		return mustCreateGroup(t, sut, name, slug)
	}

	recordHeartbeat := func(payload string) {
		req := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader([]byte(payload)))
		rec := httptest.NewRecorder()
		sut.handleAssetActions(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("expected 202 heartbeat, got %d", rec.Code)
		}
	}

	getAssetGroupID := func(assetID string) string {
		req := httptest.NewRequest(http.MethodGet, "/assets/"+assetID, nil)
		rec := httptest.NewRecorder()
		sut.handleAssetActions(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 fetching asset %s, got %d", assetID, rec.Code)
		}
		var out struct {
			Asset struct {
				GroupID string `json:"group_id"`
			} `json:"asset"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("failed to decode asset response for %s: %v", assetID, err)
		}
		return out.Asset.GroupID
	}

	groupOneID := createGroup("Edge Group", "EDGE-CASCADE")
	groupTwoID := createGroup("Core Group", "CORE-CASCADE")

	recordHeartbeat(`{
		"asset_id":"portainer-endpoint-1",
		"type":"container-host",
		"name":"edge-endpoint",
		"source":"portainer",
		"group_id":"` + groupOneID + `",
		"status":"online",
		"metadata":{"endpoint_id":"1"}
	}`)
	recordHeartbeat(`{
		"asset_id":"portainer-container-1-a",
		"type":"container",
		"name":"svc-a",
		"source":"portainer",
		"status":"online",
		"metadata":{"endpoint_id":"1"}
	}`)
	recordHeartbeat(`{
		"asset_id":"portainer-stack-10",
		"type":"stack",
		"name":"stack-1",
		"source":"portainer",
		"group_id":"` + groupOneID + `",
		"status":"online",
		"metadata":{"endpoint_id":"1"}
	}`)
	recordHeartbeat(`{
		"asset_id":"portainer-container-2-b",
		"type":"container",
		"name":"svc-b",
		"source":"portainer",
		"group_id":"` + groupOneID + `",
		"status":"online",
		"metadata":{"endpoint_id":"2"}
	}`)

	updateReq := httptest.NewRequest(
		http.MethodPatch,
		"/assets/portainer-endpoint-1",
		bytes.NewReader([]byte(`{"group_id":"`+groupTwoID+`"}`)),
	)
	updateRec := httptest.NewRecorder()
	sut.handleAssetActions(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200 updating parent host, got %d", updateRec.Code)
	}

	if got := getAssetGroupID("portainer-endpoint-1"); got != groupTwoID {
		t.Fatalf("expected parent group_id %q, got %q", groupTwoID, got)
	}
	if got := getAssetGroupID("portainer-container-1-a"); got != groupTwoID {
		t.Fatalf("expected container child group_id %q, got %q", groupTwoID, got)
	}
	if got := getAssetGroupID("portainer-stack-10"); got != groupTwoID {
		t.Fatalf("expected stack child group_id %q, got %q", groupTwoID, got)
	}
	if got := getAssetGroupID("portainer-container-2-b"); got != groupOneID {
		t.Fatalf("expected unrelated child to remain on %q, got %q", groupOneID, got)
	}
}

func TestUpdateAssetGroupCascadesToPortainerSubDevicesScopedByCollectorID(t *testing.T) {
	sut := newTestAPIServer(t)

	createGroup := func(name, slug string) string {
		return mustCreateGroup(t, sut, name, slug)
	}

	recordHeartbeat := func(payload string) {
		req := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader([]byte(payload)))
		rec := httptest.NewRecorder()
		sut.handleAssetActions(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("expected 202 heartbeat, got %d", rec.Code)
		}
	}

	getAssetGroupID := func(assetID string) string {
		req := httptest.NewRequest(http.MethodGet, "/assets/"+assetID, nil)
		rec := httptest.NewRecorder()
		sut.handleAssetActions(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 fetching asset %s, got %d", assetID, rec.Code)
		}
		var out struct {
			Asset struct {
				GroupID string `json:"group_id"`
			} `json:"asset"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("failed to decode asset response for %s: %v", assetID, err)
		}
		return out.Asset.GroupID
	}

	groupOneID := createGroup("Portainer A", "PORT-A")
	groupTwoID := createGroup("Portainer B", "PORT-B")

	recordHeartbeat(`{
		"asset_id":"portainer-endpoint-scoped-a",
		"type":"container-host",
		"name":"endpoint-a",
		"source":"portainer",
		"group_id":"` + groupOneID + `",
		"status":"online",
		"metadata":{"endpoint_id":"1","collector_id":"collector-portainer-a"}
	}`)
	recordHeartbeat(`{
		"asset_id":"portainer-container-scoped-a1",
		"type":"container",
		"name":"svc-a1",
		"source":"portainer",
		"status":"online",
		"metadata":{"endpoint_id":"1","collector_id":"collector-portainer-a"}
	}`)
	recordHeartbeat(`{
		"asset_id":"portainer-container-scoped-b1",
		"type":"container",
		"name":"svc-b1",
		"source":"portainer",
		"group_id":"` + groupOneID + `",
		"status":"online",
		"metadata":{"endpoint_id":"1","collector_id":"collector-portainer-b"}
	}`)

	updateReq := httptest.NewRequest(
		http.MethodPatch,
		"/assets/portainer-endpoint-scoped-a",
		bytes.NewReader([]byte(`{"group_id":"`+groupTwoID+`"}`)),
	)
	updateRec := httptest.NewRecorder()
	sut.handleAssetActions(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200 updating parent host, got %d", updateRec.Code)
	}

	if got := getAssetGroupID("portainer-endpoint-scoped-a"); got != groupTwoID {
		t.Fatalf("expected parent group_id %q, got %q", groupTwoID, got)
	}
	if got := getAssetGroupID("portainer-container-scoped-a1"); got != groupTwoID {
		t.Fatalf("expected same-collector child group_id %q, got %q", groupTwoID, got)
	}
	if got := getAssetGroupID("portainer-container-scoped-b1"); got != groupOneID {
		t.Fatalf("expected different-collector child to remain on %q, got %q", groupOneID, got)
	}
}

func TestUpdateAssetGroupCascadesToProxmoxSubDevices(t *testing.T) {
	sut := newTestAPIServer(t)

	createGroup := func(name, slug string) string {
		return mustCreateGroup(t, sut, name, slug)
	}

	recordHeartbeat := func(payload string) {
		req := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader([]byte(payload)))
		rec := httptest.NewRecorder()
		sut.handleAssetActions(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("expected 202 heartbeat, got %d", rec.Code)
		}
	}

	getAssetGroupID := func(assetID string) string {
		req := httptest.NewRequest(http.MethodGet, "/assets/"+assetID, nil)
		rec := httptest.NewRecorder()
		sut.handleAssetActions(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 fetching asset %s, got %d", assetID, rec.Code)
		}
		var out struct {
			Asset struct {
				GroupID string `json:"group_id"`
			} `json:"asset"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("failed to decode asset response for %s: %v", assetID, err)
		}
		return out.Asset.GroupID
	}

	groupOneID := createGroup("Compute Group", "CMP-CASCADE")
	groupTwoID := createGroup("Backup Group", "BKP-CASCADE")

	recordHeartbeat(`{
		"asset_id":"proxmox-node-pve01",
		"type":"hypervisor-node",
		"name":"pve01",
		"source":"proxmox",
		"group_id":"` + groupOneID + `",
		"status":"online",
		"metadata":{"node":"pve01"}
	}`)
	recordHeartbeat(`{
		"asset_id":"proxmox-vm-101",
		"type":"vm",
		"name":"app-vm",
		"source":"proxmox",
		"group_id":"` + groupOneID + `",
		"status":"online",
		"metadata":{"node":"pve01","vmid":"101"}
	}`)
	recordHeartbeat(`{
		"asset_id":"proxmox-ct-102",
		"type":"container",
		"name":"svc-ct",
		"source":"proxmox",
		"status":"online",
		"metadata":{"node":"pve01","vmid":"102"}
	}`)
	recordHeartbeat(`{
		"asset_id":"proxmox-vm-201",
		"type":"vm",
		"name":"other-vm",
		"source":"proxmox",
		"group_id":"` + groupOneID + `",
		"status":"online",
		"metadata":{"node":"pve02","vmid":"201"}
	}`)

	updateReq := httptest.NewRequest(
		http.MethodPatch,
		"/assets/proxmox-node-pve01",
		bytes.NewReader([]byte(`{"group_id":"`+groupTwoID+`"}`)),
	)
	updateRec := httptest.NewRecorder()
	sut.handleAssetActions(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200 updating parent host, got %d", updateRec.Code)
	}

	if got := getAssetGroupID("proxmox-node-pve01"); got != groupTwoID {
		t.Fatalf("expected parent group_id %q, got %q", groupTwoID, got)
	}
	if got := getAssetGroupID("proxmox-vm-101"); got != groupTwoID {
		t.Fatalf("expected vm child group_id %q, got %q", groupTwoID, got)
	}
	if got := getAssetGroupID("proxmox-ct-102"); got != groupTwoID {
		t.Fatalf("expected ct child group_id %q, got %q", groupTwoID, got)
	}
	if got := getAssetGroupID("proxmox-vm-201"); got != groupOneID {
		t.Fatalf("expected unrelated child to remain on %q, got %q", groupOneID, got)
	}
}

func TestUpdateAssetGroupCascadesToDockerSubDevicesByAgentID(t *testing.T) {
	sut := newTestAPIServer(t)

	createGroup := func(name, slug string) string {
		return mustCreateGroup(t, sut, name, slug)
	}

	recordHeartbeat := func(payload string) {
		req := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader([]byte(payload)))
		rec := httptest.NewRecorder()
		sut.handleAssetActions(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("expected 202 heartbeat, got %d", rec.Code)
		}
	}

	getAssetGroupID := func(assetID string) string {
		req := httptest.NewRequest(http.MethodGet, "/assets/"+assetID, nil)
		rec := httptest.NewRecorder()
		sut.handleAssetActions(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 fetching asset %s, got %d", assetID, rec.Code)
		}
		var out struct {
			Asset struct {
				GroupID string `json:"group_id"`
			} `json:"asset"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("failed to decode asset response for %s: %v", assetID, err)
		}
		return out.Asset.GroupID
	}

	groupOneID := createGroup("Docker Group A", "DOCK-A")
	groupTwoID := createGroup("Docker Group B", "DOCK-B")

	recordHeartbeat(`{
		"asset_id":"docker-host-agent-a",
		"type":"container-host",
		"name":"docker-agent-a",
		"source":"docker",
		"group_id":"` + groupOneID + `",
		"status":"online",
		"metadata":{"agent_id":"agent-a"}
	}`)
	recordHeartbeat(`{
		"asset_id":"docker-ct-agent-a-web",
		"type":"docker-container",
		"name":"web",
		"source":"docker",
		"status":"online",
		"metadata":{"agent_id":"agent-a"}
	}`)
	recordHeartbeat(`{
		"asset_id":"docker-ct-agent-b-db",
		"type":"docker-container",
		"name":"db",
		"source":"docker",
		"group_id":"` + groupOneID + `",
		"status":"online",
		"metadata":{"agent_id":"agent-b"}
	}`)

	updateReq := httptest.NewRequest(
		http.MethodPatch,
		"/assets/docker-host-agent-a",
		bytes.NewReader([]byte(`{"group_id":"`+groupTwoID+`"}`)),
	)
	updateRec := httptest.NewRecorder()
	sut.handleAssetActions(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200 updating docker host, got %d", updateRec.Code)
	}

	if got := getAssetGroupID("docker-host-agent-a"); got != groupTwoID {
		t.Fatalf("expected host group_id %q, got %q", groupTwoID, got)
	}
	if got := getAssetGroupID("docker-ct-agent-a-web"); got != groupTwoID {
		t.Fatalf("expected attached docker child group_id %q, got %q", groupTwoID, got)
	}
	if got := getAssetGroupID("docker-ct-agent-b-db"); got != groupOneID {
		t.Fatalf("expected unrelated docker child to remain on %q, got %q", groupOneID, got)
	}
}

func TestUpdateAssetGroupCascadesToHomeAssistantEntitiesByCollectorID(t *testing.T) {
	sut := newTestAPIServer(t)

	createGroup := func(name, slug string) string {
		return mustCreateGroup(t, sut, name, slug)
	}

	recordHeartbeat := func(payload string) {
		req := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader([]byte(payload)))
		rec := httptest.NewRecorder()
		sut.handleAssetActions(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("expected 202 heartbeat, got %d", rec.Code)
		}
	}

	getAssetGroupID := func(assetID string) string {
		req := httptest.NewRequest(http.MethodGet, "/assets/"+assetID, nil)
		rec := httptest.NewRecorder()
		sut.handleAssetActions(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 fetching asset %s, got %d", assetID, rec.Code)
		}
		var out struct {
			Asset struct {
				GroupID string `json:"group_id"`
			} `json:"asset"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("failed to decode asset response for %s: %v", assetID, err)
		}
		return out.Asset.GroupID
	}

	groupOneID := createGroup("HA Group A", "HA-A")
	groupTwoID := createGroup("HA Group B", "HA-B")

	recordHeartbeat(`{
		"asset_id":"ha-cluster-group-a",
		"type":"connector-cluster",
		"name":"ha-cluster-group-a",
		"source":"homeassistant",
		"group_id":"` + groupOneID + `",
		"status":"online",
		"metadata":{"collector_id":"collector-ha-a"}
	}`)
	recordHeartbeat(`{
		"asset_id":"ha-entity-group-a1",
		"type":"ha-entity",
		"name":"light.a1",
		"source":"homeassistant",
		"status":"online",
		"metadata":{"collector_id":"collector-ha-a","entity_id":"light.a1"}
	}`)
	recordHeartbeat(`{
		"asset_id":"ha-entity-group-b1",
		"type":"ha-entity",
		"name":"light.b1",
		"source":"homeassistant",
		"group_id":"` + groupOneID + `",
		"status":"online",
		"metadata":{"collector_id":"collector-ha-b","entity_id":"light.b1"}
	}`)

	updateReq := httptest.NewRequest(
		http.MethodPatch,
		"/assets/ha-cluster-group-a",
		bytes.NewReader([]byte(`{"group_id":"`+groupTwoID+`"}`)),
	)
	updateRec := httptest.NewRecorder()
	sut.handleAssetActions(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200 updating homeassistant cluster, got %d", updateRec.Code)
	}

	if got := getAssetGroupID("ha-cluster-group-a"); got != groupTwoID {
		t.Fatalf("expected cluster group_id %q, got %q", groupTwoID, got)
	}
	if got := getAssetGroupID("ha-entity-group-a1"); got != groupTwoID {
		t.Fatalf("expected same-collector entity group_id %q, got %q", groupTwoID, got)
	}
	if got := getAssetGroupID("ha-entity-group-b1"); got != groupOneID {
		t.Fatalf("expected different-collector entity to remain on %q, got %q", groupOneID, got)
	}
}

func TestUpdateAssetGroupCascadesToFutureCollectorClusterChildrenByCollectorID(t *testing.T) {
	sut := newTestAPIServer(t)

	createGroup := func(name, slug string) string {
		return mustCreateGroup(t, sut, name, slug)
	}

	recordHeartbeat := func(payload string) {
		req := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader([]byte(payload)))
		rec := httptest.NewRecorder()
		sut.handleAssetActions(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("expected 202 heartbeat, got %d", rec.Code)
		}
	}

	getAssetGroupID := func(assetID string) string {
		req := httptest.NewRequest(http.MethodGet, "/assets/"+assetID, nil)
		rec := httptest.NewRecorder()
		sut.handleAssetActions(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 fetching asset %s, got %d", assetID, rec.Code)
		}
		var out struct {
			Asset struct {
				GroupID string `json:"group_id"`
			} `json:"asset"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("failed to decode asset response for %s: %v", assetID, err)
		}
		return out.Asset.GroupID
	}

	groupOneID := createGroup("Future API Group A", "FAPI-A")
	groupTwoID := createGroup("Future API Group B", "FAPI-B")

	recordHeartbeat(`{
		"asset_id":"future-cluster-group-a",
		"type":"connector-cluster",
		"name":"future-cluster-group-a",
		"source":"futureapi",
		"group_id":"` + groupOneID + `",
		"status":"online",
		"metadata":{"collector_id":"collector-future-a"}
	}`)
	recordHeartbeat(`{
		"asset_id":"future-service-group-a",
		"type":"service",
		"name":"future-service-group-a",
		"source":"futureapi",
		"status":"online",
		"metadata":{"collector_id":"collector-future-a"}
	}`)
	recordHeartbeat(`{
		"asset_id":"future-service-group-b",
		"type":"service",
		"name":"future-service-group-b",
		"source":"futureapi",
		"group_id":"` + groupOneID + `",
		"status":"online",
		"metadata":{"collector_id":"collector-future-b"}
	}`)

	updateReq := httptest.NewRequest(
		http.MethodPatch,
		"/assets/future-cluster-group-a",
		bytes.NewReader([]byte(`{"group_id":"`+groupTwoID+`"}`)),
	)
	updateRec := httptest.NewRecorder()
	sut.handleAssetActions(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200 updating future collector cluster, got %d", updateRec.Code)
	}

	if got := getAssetGroupID("future-cluster-group-a"); got != groupTwoID {
		t.Fatalf("expected cluster group_id %q, got %q", groupTwoID, got)
	}
	if got := getAssetGroupID("future-service-group-a"); got != groupTwoID {
		t.Fatalf("expected same-collector child group_id %q, got %q", groupTwoID, got)
	}
	if got := getAssetGroupID("future-service-group-b"); got != groupOneID {
		t.Fatalf("expected different-collector child to remain on %q, got %q", groupOneID, got)
	}
}

func TestUpdateAssetRejectsUnknownGroup(t *testing.T) {
	sut := newTestAPIServer(t)

	assetPayload := []byte(`{"asset_id":"lab-host-update-unknown-group","type":"host","name":"Host","source":"agent","status":"online","platform":"linux"}`)
	assetReq := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(assetPayload))
	assetRec := httptest.NewRecorder()
	sut.handleAssetActions(assetRec, assetReq)
	if assetRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", assetRec.Code)
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/assets/lab-host-update-unknown-group", bytes.NewReader([]byte(`{"group_id":"group_missing"}`)))
	updateRec := httptest.NewRecorder()
	sut.handleAssetActions(updateRec, updateReq)
	if updateRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", updateRec.Code)
	}
}

func TestDeleteGroupUnlinksAssets(t *testing.T) {
	sut := newTestAPIServer(t)

	groupID := mustCreateGroup(t, sut, "Office Lab", "office")

	assetPayload := []byte(`{"asset_id":"lab-host-delete-group","type":"host","name":"Lab Host Delete Group","source":"agent","group_id":"` + groupID + `","status":"online","platform":"linux"}`)
	assetReq := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(assetPayload))
	assetRec := httptest.NewRecorder()
	sut.handleAssetActions(assetRec, assetReq)
	if assetRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", assetRec.Code)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/groups/"+groupID, nil)
	deleteRec := httptest.NewRecorder()
	sut.handleGroupActions(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", deleteRec.Code)
	}

	getGroupReq := httptest.NewRequest(http.MethodGet, "/groups/"+groupID, nil)
	getGroupRec := httptest.NewRecorder()
	sut.handleGroupActions(getGroupRec, getGroupReq)
	if getGroupRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", getGroupRec.Code)
	}

	getAssetReq := httptest.NewRequest(http.MethodGet, "/assets/lab-host-delete-group", nil)
	getAssetRec := httptest.NewRecorder()
	sut.handleAssetActions(getAssetRec, getAssetReq)
	if getAssetRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getAssetRec.Code)
	}

	var getAssetResponse struct {
		Asset struct {
			GroupID string `json:"group_id"`
		} `json:"asset"`
	}
	if err := json.Unmarshal(getAssetRec.Body.Bytes(), &getAssetResponse); err != nil {
		t.Fatalf("failed to decode asset response: %v", err)
	}
	if getAssetResponse.Asset.GroupID != "" {
		t.Fatalf("expected empty group_id after group delete, got %s", getAssetResponse.Asset.GroupID)
	}
}

func TestGroupGeoFieldsPersist(t *testing.T) {
	sut := newTestAPIServer(t)

	payload := []byte(`{"name":"Geo Lab","slug":"geo","location":"Austin","latitude":30.2672,"longitude":-97.7431}`)
	createReq := httptest.NewRequest(http.MethodPost, "/groups", bytes.NewReader(payload))
	createRec := httptest.NewRecorder()
	sut.handleGroups(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRec.Code)
	}

	var response struct {
		Group struct {
			ID        string   `json:"id"`
			Latitude  *float64 `json:"latitude"`
			Longitude *float64 `json:"longitude"`
		} `json:"group"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if response.Group.ID == "" {
		t.Fatalf("expected group id")
	}
	if response.Group.Latitude == nil || response.Group.Longitude == nil {
		t.Fatalf("expected latitude/longitude values")
	}
}

func TestAssetsAndMetricsFilterByGroup(t *testing.T) {
	sut := newTestAPIServer(t)

	createGroup := func(name, slug string) string {
		return mustCreateGroup(t, sut, name, slug)
	}

	groupOneID := createGroup("Primary Lab", "PRI")
	groupTwoID := createGroup("Secondary Lab", "SEC")

	heartbeatOne := []byte(`{"asset_id":"group-filter-1","type":"host","name":"Group Filter One","source":"agent","group_id":"` + groupOneID + `","status":"online","platform":"linux","metadata":{"cpu_percent":"11"}}`)
	heartbeatTwo := []byte(`{"asset_id":"group-filter-2","type":"host","name":"Group Filter Two","source":"agent","group_id":"` + groupTwoID + `","status":"online","platform":"linux","metadata":{"cpu_percent":"22"}}`)

	for _, payload := range [][]byte{heartbeatOne, heartbeatTwo} {
		req := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(payload))
		rec := httptest.NewRecorder()
		sut.handleAssetActions(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("expected 202, got %d", rec.Code)
		}
	}

	assetsReq := httptest.NewRequest(http.MethodGet, "/assets?group_id="+groupOneID, nil)
	assetsRec := httptest.NewRecorder()
	sut.handleAssets(assetsRec, assetsReq)
	if assetsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", assetsRec.Code)
	}
	var assetsResponse struct {
		Assets []struct {
			ID string `json:"id"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(assetsRec.Body.Bytes(), &assetsResponse); err != nil {
		t.Fatalf("failed to decode assets response: %v", err)
	}
	if len(assetsResponse.Assets) != 1 || assetsResponse.Assets[0].ID != "group-filter-1" {
		t.Fatalf("expected only group-filter-1 asset, got %#v", assetsResponse.Assets)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics/overview?group_id="+groupOneID+"&window=15m", nil)
	metricsRec := httptest.NewRecorder()
	sut.handleMetricsOverview(metricsRec, metricsReq)
	if metricsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", metricsRec.Code)
	}
	var metricsResponse struct {
		Assets []struct {
			AssetID string `json:"asset_id"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(metricsRec.Body.Bytes(), &metricsResponse); err != nil {
		t.Fatalf("failed to decode metrics response: %v", err)
	}
	if len(metricsResponse.Assets) != 1 || metricsResponse.Assets[0].AssetID != "group-filter-1" {
		t.Fatalf("expected only group-filter-1 metrics, got %#v", metricsResponse.Assets)
	}
}

func TestAssetsFilterByTag(t *testing.T) {
	sut := newTestAPIServer(t)

	recordHeartbeat := func(payload []byte) {
		req := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(payload))
		rec := httptest.NewRecorder()
		sut.handleAssetActions(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("expected 202, got %d", rec.Code)
		}
	}

	recordHeartbeat([]byte(`{"asset_id":"tag-filter-1","type":"host","name":"Tag Filter One","source":"agent","status":"online","platform":"linux"}`))
	recordHeartbeat([]byte(`{"asset_id":"tag-filter-2","type":"host","name":"Tag Filter Two","source":"agent","status":"online","platform":"linux"}`))

	assignTags := func(assetID, payload string) {
		req := httptest.NewRequest(http.MethodPatch, "/assets/"+assetID, bytes.NewReader([]byte(payload)))
		rec := httptest.NewRecorder()
		sut.handleAssetActions(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 assigning tags to %s, got %d", assetID, rec.Code)
		}
	}

	assignTags("tag-filter-1", `{"tags":["edge","prod"]}`)
	assignTags("tag-filter-2", `{"tags":["dev"]}`)

	req := httptest.NewRequest(http.MethodGet, "/assets?tag=edge", nil)
	rec := httptest.NewRecorder()
	sut.handleAssets(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var out struct {
		Assets []struct {
			ID string `json:"id"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("failed to decode assets response: %v", err)
	}
	if len(out.Assets) != 1 || out.Assets[0].ID != "tag-filter-1" {
		t.Fatalf("expected only tag-filter-1, got %#v", out.Assets)
	}
}

func TestAssetsGroupOperationsReturnServiceUnavailableWithoutGroupStore(t *testing.T) {
	sut := newTestAPIServer(t)

	heartbeatPayload := []byte(`{"asset_id":"group-store-guard-node","type":"host","name":"Group Store Guard Node","source":"agent","status":"online","platform":"linux"}`)
	heartbeatReq := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(heartbeatPayload))
	heartbeatRec := httptest.NewRecorder()
	sut.handleAssetActions(heartbeatRec, heartbeatReq)
	if heartbeatRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", heartbeatRec.Code)
	}

	sut.groupStore = nil

	filterReq := httptest.NewRequest(http.MethodGet, "/assets?group_id=group-1", nil)
	filterRec := httptest.NewRecorder()
	sut.handleAssets(filterRec, filterReq)
	if filterRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for group-filtered assets when group store is unavailable, got %d", filterRec.Code)
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/assets/group-store-guard-node", bytes.NewReader([]byte(`{"group_id":"group-1"}`)))
	updateRec := httptest.NewRecorder()
	sut.handleAssetActions(updateRec, updateReq)
	if updateRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for group assignment update when group store is unavailable, got %d", updateRec.Code)
	}
}
