package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/persistence"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIncidentCreateListGetUpdateAndLinkAlert(t *testing.T) {
	sut := newTestAPIServer(t)

	groupReq := httptest.NewRequest(http.MethodPost, "/groups", bytes.NewReader([]byte(`{"name":"Incident Lab","slug":"incident-lab"}`)))
	groupRec := httptest.NewRecorder()
	sut.handleGroups(groupRec, groupReq)
	if groupRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", groupRec.Code)
	}
	var sitePayload struct {
		Group struct {
			ID string `json:"id"`
		} `json:"group"`
	}
	if err := json.Unmarshal(groupRec.Body.Bytes(), &sitePayload); err != nil {
		t.Fatalf("failed to decode group payload: %v", err)
	}

	assetPayload := []byte(`{"asset_id":"incident-node-1","type":"host","name":"Incident Node 1","source":"agent","group_id":"` + sitePayload.Group.ID + `","status":"online","platform":"linux"}`)
	assetReq := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(assetPayload))
	assetRec := httptest.NewRecorder()
	sut.handleAssetActions(assetRec, assetReq)
	if assetRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", assetRec.Code)
	}

	createRulePayload := []byte(`{
		"name":"Incident Link Rule",
		"description":"linked rule",
		"kind":"metric_threshold",
		"severity":"high",
		"target_scope":"asset",
		"condition":{"metric":"cpu_used_percent","operator":">=","threshold":90},
		"targets":[{"asset_id":"incident-node-1"}]
	}`)
	createRuleReq := httptest.NewRequest(http.MethodPost, "/alerts/rules", bytes.NewReader(createRulePayload))
	createRuleRec := httptest.NewRecorder()
	sut.handleAlertRules(createRuleRec, createRuleReq)
	if createRuleRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating alert rule, got %d", createRuleRec.Code)
	}
	var rulePayload struct {
		Rule alerts.Rule `json:"rule"`
	}
	if err := json.Unmarshal(createRuleRec.Body.Bytes(), &rulePayload); err != nil {
		t.Fatalf("failed to decode rule payload: %v", err)
	}

	createIncidentPayload := []byte(`{
		"title":"Primary host instability",
		"summary":"CPU pressure and intermittent service impact",
		"severity":"high",
		"source":"manual",
		"group_id":"` + sitePayload.Group.ID + `",
		"primary_asset_id":"incident-node-1",
		"assignee":"owner",
		"metadata":{"service":"media-stack"}
	}`)
	createIncidentReq := httptest.NewRequest(http.MethodPost, "/incidents", bytes.NewReader(createIncidentPayload))
	createIncidentRec := httptest.NewRecorder()
	sut.handleIncidents(createIncidentRec, createIncidentReq)
	if createIncidentRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createIncidentRec.Code)
	}
	var createIncidentResponse struct {
		Incident incidents.Incident `json:"incident"`
	}
	if err := json.Unmarshal(createIncidentRec.Body.Bytes(), &createIncidentResponse); err != nil {
		t.Fatalf("failed to decode incident create response: %v", err)
	}
	if createIncidentResponse.Incident.Status != incidents.StatusOpen {
		t.Fatalf("expected open status, got %s", createIncidentResponse.Incident.Status)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/incidents?status=open&group_id="+sitePayload.Group.ID, nil)
	listRec := httptest.NewRecorder()
	sut.handleIncidents(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/incidents/"+createIncidentResponse.Incident.ID, nil)
	getRec := httptest.NewRecorder()
	sut.handleIncidentActions(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}

	updatePayload := []byte(`{"status":"investigating","assignee":"oncall"}`)
	updateReq := httptest.NewRequest(http.MethodPatch, "/incidents/"+createIncidentResponse.Incident.ID, bytes.NewReader(updatePayload))
	updateRec := httptest.NewRecorder()
	sut.handleIncidentActions(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", updateRec.Code)
	}
	var updateResponse struct {
		Incident incidents.Incident `json:"incident"`
	}
	if err := json.Unmarshal(updateRec.Body.Bytes(), &updateResponse); err != nil {
		t.Fatalf("failed to decode incident update response: %v", err)
	}
	if updateResponse.Incident.Status != incidents.StatusInvestigating {
		t.Fatalf("expected investigating status, got %s", updateResponse.Incident.Status)
	}

	linkPayload := []byte(`{"alert_rule_id":"` + rulePayload.Rule.ID + `","link_type":"trigger"}`)
	linkReq := httptest.NewRequest(http.MethodPost, "/incidents/"+createIncidentResponse.Incident.ID+"/link-alert", bytes.NewReader(linkPayload))
	linkRec := httptest.NewRecorder()
	sut.handleIncidentActions(linkRec, linkReq)
	if linkRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", linkRec.Code)
	}

	linksReq := httptest.NewRequest(http.MethodGet, "/incidents/"+createIncidentResponse.Incident.ID+"/alerts?limit=10", nil)
	linksRec := httptest.NewRecorder()
	sut.handleIncidentActions(linksRec, linksReq)
	if linksRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", linksRec.Code)
	}
	var linksResponse struct {
		Links []incidents.AlertLink `json:"links"`
	}
	if err := json.Unmarshal(linksRec.Body.Bytes(), &linksResponse); err != nil {
		t.Fatalf("failed to decode incident alert links response: %v", err)
	}
	if len(linksResponse.Links) != 1 {
		t.Fatalf("expected 1 alert link, got %d", len(linksResponse.Links))
	}
}

func TestIncidentInvalidStatusTransitionRejected(t *testing.T) {
	sut := newTestAPIServer(t)

	createIncidentReq := httptest.NewRequest(http.MethodPost, "/incidents", bytes.NewReader([]byte(`{
		"title":"Transition Test",
		"severity":"medium"
	}`)))
	createIncidentRec := httptest.NewRecorder()
	sut.handleIncidents(createIncidentRec, createIncidentReq)
	if createIncidentRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createIncidentRec.Code)
	}

	var createIncidentResponse struct {
		Incident incidents.Incident `json:"incident"`
	}
	if err := json.Unmarshal(createIncidentRec.Body.Bytes(), &createIncidentResponse); err != nil {
		t.Fatalf("failed to decode incident create response: %v", err)
	}

	invalidUpdateReq := httptest.NewRequest(http.MethodPatch, "/incidents/"+createIncidentResponse.Incident.ID, bytes.NewReader([]byte(`{"status":"mitigated"}`)))
	invalidUpdateRec := httptest.NewRecorder()
	sut.handleIncidentActions(invalidUpdateRec, invalidUpdateReq)
	if invalidUpdateRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", invalidUpdateRec.Code)
	}
}

func TestAlertSuppressedPendingDoesNotAutoCreateIncident(t *testing.T) {
	sut := newTestAPIServer(t)
	ctx := context.Background()

	seedAssetViaHeartbeat(t, sut, "suppressed-autoincident-node-1", "SAINC")
	seedMetricSamples(t, sut, "suppressed-autoincident-node-1", "cpu_used_percent", 99.0)

	rule := mustCreateAlertRule(t, sut, alerts.CreateRuleRequest{
		Name:        "Suppressed Auto Incident Guard",
		Kind:        alerts.RuleKindMetricThreshold,
		Severity:    alerts.SeverityCritical,
		TargetScope: alerts.TargetScopeAsset,
		Condition:   map[string]any{"metric": "cpu_used_percent", "operator": ">", "threshold": float64(90)},
		Labels:      map[string]string{"env": "prod", "team": "platform"},
		Targets:     []alerts.RuleTargetInput{{AssetID: "suppressed-autoincident-node-1"}},
	})

	now := time.Now().UTC()
	_, err := sut.alertInstanceStore.CreateAlertSilence(alerts.CreateSilenceRequest{
		Matchers:  map[string]string{"env": "prod", "team": "platform"},
		Reason:    "Maintenance suppression",
		CreatedBy: "owner",
		StartsAt:  now.Add(-1 * time.Hour),
		EndsAt:    now.Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("failed to create silence: %v", err)
	}

	// First evaluation creates suppressed pending instance.
	sut.evaluateSingleRule(ctx, rule, nil)
	pendingInstances, err := sut.alertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{
		RuleID: rule.ID,
		Status: alerts.InstanceStatusPending,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("failed to list pending instances: %v", err)
	}
	if len(pendingInstances) != 1 {
		t.Fatalf("expected exactly 1 pending suppressed instance, got %d", len(pendingInstances))
	}

	// Simulate prolonged suppression duration, then re-evaluate.
	backdateAlertInstanceStartedAt(t, sut, pendingInstances[0].ID, time.Now().UTC().Add(-6*time.Minute))
	sut.evaluateSingleRule(ctx, rule, nil)

	// Suppressed/pending instances must not trigger auto incident creation.
	autoIncidents, err := sut.incidentStore.ListIncidents(persistence.IncidentFilter{
		Source: incidents.SourceAlertAuto,
		Limit:  100,
	})
	if err != nil {
		t.Fatalf("failed to list auto incidents: %v", err)
	}
	if len(autoIncidents) != 0 {
		t.Fatalf("expected 0 auto incidents for suppressed pending alerts, got %d", len(autoIncidents))
	}
}

// TestAlertAutoIncidentTiming creates a critical alert that has been firing for
// >5min, verifies an incident is auto-created, and ensures no duplicate incident
// on re-evaluation.
func TestAlertAutoIncidentTiming(t *testing.T) {
	sut := newTestAPIServer(t)
	ctx := context.Background()

	seedAssetViaHeartbeat(t, sut, "autoincident-node-1", "AUTOINC")
	seedMetricSamples(t, sut, "autoincident-node-1", "cpu_used_percent", 99.0)

	// Create critical rule.
	rule := mustCreateAlertRule(t, sut, alerts.CreateRuleRequest{
		Name:        "Auto Incident CPU Rule",
		Kind:        alerts.RuleKindMetricThreshold,
		Severity:    alerts.SeverityCritical,
		TargetScope: alerts.TargetScopeAsset,
		Condition:   map[string]any{"metric": "cpu_used_percent", "operator": ">", "threshold": float64(90)},
		Labels:      map[string]string{"env": "prod"},
		Targets:     []alerts.RuleTargetInput{{AssetID: "autoincident-node-1"}},
	})

	// First evaluation creates the firing instance.
	sut.evaluateSingleRule(ctx, rule, nil)

	// Verify instance was created in firing status.
	instances, err := sut.alertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{
		RuleID: rule.ID,
		Status: alerts.InstanceStatusFiring,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("failed to list instances: %v", err)
	}
	if len(instances) == 0 {
		t.Fatalf("expected a firing instance after first evaluation")
	}

	// At this point the instance just started, so it should NOT have created
	// an incident yet (< 5 min). Re-evaluate to confirm.
	sut.evaluateSingleRule(ctx, rule, nil)

	autoIncidents, err := sut.incidentStore.ListIncidents(persistence.IncidentFilter{
		Source: incidents.SourceAlertAuto,
		Limit:  100,
	})
	if err != nil {
		t.Fatalf("failed to list auto incidents: %v", err)
	}
	if len(autoIncidents) != 0 {
		t.Fatalf("expected 0 auto incidents before 5min, got %d", len(autoIncidents))
	}

	// Backdate the instance's StartedAt to >5 min ago to simulate elapsed time.
	// We do this by directly manipulating the store.
	inst := instances[0]
	backdateAlertInstanceStartedAt(t, sut, inst.ID, time.Now().UTC().Add(-6*time.Minute))

	// Re-evaluate -- now >5min elapsed so an incident should be auto-created.
	sut.evaluateSingleRule(ctx, rule, nil)

	autoIncidents, err = sut.incidentStore.ListIncidents(persistence.IncidentFilter{
		Source: incidents.SourceAlertAuto,
		Limit:  100,
	})
	if err != nil {
		t.Fatalf("failed to list auto incidents: %v", err)
	}
	if len(autoIncidents) != 1 {
		t.Fatalf("expected exactly 1 auto-created incident, got %d", len(autoIncidents))
	}
	if autoIncidents[0].Severity != incidents.SeverityCritical {
		t.Fatalf("expected critical incident, got %s", autoIncidents[0].Severity)
	}

	// Re-evaluate again -- should NOT create a duplicate incident.
	sut.evaluateSingleRule(ctx, rule, nil)

	autoIncidents, err = sut.incidentStore.ListIncidents(persistence.IncidentFilter{
		Source: incidents.SourceAlertAuto,
		Limit:  100,
	})
	if err != nil {
		t.Fatalf("failed to list auto incidents after re-evaluation: %v", err)
	}
	if len(autoIncidents) != 1 {
		t.Fatalf("expected still 1 auto-created incident (no duplicate), got %d", len(autoIncidents))
	}
}
