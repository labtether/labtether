package alerting

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleIncidentsListGroupFilterReturnsServiceUnavailableWithoutGroupStore(t *testing.T) {
	deps := newTestAlertingDeps(t)
	deps.GroupStore = nil

	req := httptest.NewRequest(http.MethodGet, "/incidents?group_id=group-1", nil)
	rec := httptest.NewRecorder()
	deps.HandleIncidents(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when group store is unavailable, got %d", rec.Code)
	}
}

func TestHandleIncidentsCreateReturnsServiceUnavailableWithoutGroupStore(t *testing.T) {
	deps := newTestAlertingDeps(t)
	deps.GroupStore = nil

	payload := []byte(`{"title":"Group Incident","severity":"high","group_id":"group-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/incidents", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	deps.HandleIncidents(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when group store is unavailable, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleIncidentActionsUpdateReturnsServiceUnavailableWithoutGroupStore(t *testing.T) {
	deps := newTestAlertingDeps(t)

	createPayload := []byte(`{"title":"Update Group Incident","severity":"medium"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/incidents", bytes.NewReader(createPayload))
	createRec := httptest.NewRecorder()
	deps.HandleIncidents(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating incident, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	var created struct {
		Incident struct {
			ID string `json:"id"`
		} `json:"incident"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode incident create response: %v", err)
	}
	if created.Incident.ID == "" {
		t.Fatalf("expected incident id")
	}

	deps.GroupStore = nil

	updatePayload := []byte(`{"group_id":"group-1"}`)
	updateReq := httptest.NewRequest(http.MethodPatch, "/incidents/"+created.Incident.ID, bytes.NewReader(updatePayload))
	updateRec := httptest.NewRecorder()
	deps.HandleIncidentActions(updateRec, updateReq)

	if updateRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when group store is unavailable, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}
}

func TestHandleIncidentActionsDeleteRemovesIncident(t *testing.T) {
	deps := newTestAlertingDeps(t)

	createPayload := []byte(`{"title":"Delete Incident","severity":"medium"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/incidents", bytes.NewReader(createPayload))
	createRec := httptest.NewRecorder()
	deps.HandleIncidents(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating incident, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	var created struct {
		Incident struct {
			ID string `json:"id"`
		} `json:"incident"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode incident create response: %v", err)
	}
	if created.Incident.ID == "" {
		t.Fatalf("expected incident id")
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/incidents/"+created.Incident.ID, nil)
	deleteRec := httptest.NewRecorder()
	deps.HandleIncidentActions(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200 deleting incident, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/incidents/"+created.Incident.ID, nil)
	getRec := httptest.NewRecorder()
	deps.HandleIncidentActions(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after deleting incident, got %d", getRec.Code)
	}
}
