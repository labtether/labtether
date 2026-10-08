package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
