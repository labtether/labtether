package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/incidents"
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

func TestHandleIncidentLinkAlertUsesAuthenticatedActor(t *testing.T) {
	deps := newTestAlertingDeps(t)

	createReq := httptest.NewRequest(
		http.MethodPost,
		"/incidents",
		bytes.NewReader([]byte(`{"title":"Actor Attribution","severity":"medium"}`)),
	)
	createRec := httptest.NewRecorder()
	deps.HandleIncidents(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create incident: got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Incident incidents.Incident `json:"incident"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	linkReq := httptest.NewRequest(
		http.MethodPost,
		"/incidents/"+created.Incident.ID+"/link-alert",
		bytes.NewReader([]byte(`{"alert_fingerprint":"fingerprint-1","link_type":"related","created_by":"spoofed"}`)),
	)
	linkReq = linkReq.WithContext(apiv2.ContextWithPrincipal(context.Background(), "operator-1", "operator"))
	linkRec := httptest.NewRecorder()
	deps.HandleIncidentActions(linkRec, linkReq)
	if linkRec.Code != http.StatusCreated {
		t.Fatalf("link alert: got %d body=%s", linkRec.Code, linkRec.Body.String())
	}
	var linked struct {
		Link incidents.AlertLink `json:"link"`
	}
	if err := json.Unmarshal(linkRec.Body.Bytes(), &linked); err != nil {
		t.Fatal(err)
	}
	if linked.Link.CreatedBy != "operator-1" {
		t.Fatalf("created_by=%q, want authenticated actor", linked.Link.CreatedBy)
	}
}

func TestHandleIncidentsCreateUsesAuthenticatedActor(t *testing.T) {
	deps := newTestAlertingDeps(t)
	req := httptest.NewRequest(
		http.MethodPost,
		"/incidents",
		bytes.NewReader([]byte(`{"title":"Actor Attribution","severity":"medium","created_by":"spoofed"}`)),
	)
	req = req.WithContext(apiv2.ContextWithPrincipal(context.Background(), "operator-1", "operator"))
	rec := httptest.NewRecorder()
	deps.HandleIncidents(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create incident: got %d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Incident incidents.Incident `json:"incident"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Incident.CreatedBy != "operator-1" {
		t.Fatalf("created_by=%q, want authenticated actor", response.Incident.CreatedBy)
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
