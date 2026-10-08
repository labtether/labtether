package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
