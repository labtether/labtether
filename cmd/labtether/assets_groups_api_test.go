package main

import (
	"bytes"
	"encoding/json"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
	"net/http"
	"net/http/httptest"
	"testing"
)

type trackingGroupAssetStore struct {
	*persistence.MemoryAssetStore
	listAssetsCalls        int
	listAssetsByGroupCalls int
}

func (s *trackingGroupAssetStore) ListAssets() ([]assets.Asset, error) {
	s.listAssetsCalls++
	return s.MemoryAssetStore.ListAssets()
}

func (s *trackingGroupAssetStore) ListAssetsByGroup(groupID string) ([]assets.Asset, error) {
	s.listAssetsByGroupCalls++
	return s.MemoryAssetStore.ListAssetsByGroup(groupID)
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

func TestHandleAssetsUsesGroupedStorePathWhenFilteringByGroup(t *testing.T) {
	sut := newTestAPIServer(t)
	trackingStore := &trackingGroupAssetStore{MemoryAssetStore: persistence.NewMemoryAssetStore()}
	sut.assetStore = trackingStore

	groupOneID := mustCreateGroup(t, sut, "Garage Lab", "garage-fast-path")
	groupTwoID := mustCreateGroup(t, sut, "Office Lab", "office-fast-path")

	if _, err := trackingStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "asset-g1",
		Name:    "Group One",
		Type:    "host",
		Source:  "agent",
		GroupID: groupOneID,
		Status:  "online",
	}); err != nil {
		t.Fatalf("failed to seed group-one asset: %v", err)
	}
	if _, err := trackingStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "asset-g2",
		Name:    "Group Two",
		Type:    "host",
		Source:  "agent",
		GroupID: groupTwoID,
		Status:  "online",
	}); err != nil {
		t.Fatalf("failed to seed group-two asset: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/assets?group_id="+groupOneID, nil)
	rec := httptest.NewRecorder()
	sut.handleAssets(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if trackingStore.listAssetsCalls != 0 {
		t.Fatalf("expected grouped request to avoid ListAssets, got %d calls", trackingStore.listAssetsCalls)
	}
	if trackingStore.listAssetsByGroupCalls != 1 {
		t.Fatalf("expected one ListAssetsByGroup call, got %d", trackingStore.listAssetsByGroupCalls)
	}

	var resp struct {
		Assets []struct {
			ID string `json:"id"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode assets response: %v", err)
	}
	if len(resp.Assets) != 1 || resp.Assets[0].ID != "asset-g1" {
		t.Fatalf("expected only group-one asset, got %#v", resp.Assets)
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
