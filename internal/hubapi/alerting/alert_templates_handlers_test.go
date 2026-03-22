package alerting

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubapi/collectors"
)

func TestHandleAlertTemplatesListsMobileTemplates(t *testing.T) {
	deps := newTestAlertingDeps(t)
	deps.WebServiceHealthLogSource = collectors.WebServiceHealthLogSource

	req := httptest.NewRequest(http.MethodGet, "/alerts/templates", nil)
	rec := httptest.NewRecorder()

	deps.HandleAlertTemplates(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload struct {
		Templates []AlertRuleTemplateResponse `json:"templates"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode template list response: %v", err)
	}
	if len(payload.Templates) < 9 {
		t.Fatalf("expected at least 9 templates, got %d", len(payload.Templates))
	}

	ids := map[string]struct{}{}
	for _, template := range payload.Templates {
		ids[template.ID] = struct{}{}
	}
	if _, ok := ids["mobile.reconnect_storm"]; !ok {
		t.Fatalf("expected mobile.reconnect_storm template")
	}
	if _, ok := ids["starter.agent_offline"]; !ok {
		t.Fatalf("expected starter.agent_offline template")
	}
	if _, ok := ids["starter.cpu_saturation"]; !ok {
		t.Fatalf("expected starter.cpu_saturation template")
	}
	if _, ok := ids["starter.disk_nearly_full"]; !ok {
		t.Fatalf("expected starter.disk_nearly_full template")
	}
	if _, ok := ids["mobile.api_error_burst"]; !ok {
		t.Fatalf("expected mobile.api_error_burst template")
	}
	if _, ok := ids["services.web.down_transition_burst"]; !ok {
		t.Fatalf("expected services.web.down_transition_burst template")
	}
	if _, ok := ids["services.web.uptime_drop"]; !ok {
		t.Fatalf("expected services.web.uptime_drop template")
	}
}

func TestHandleAlertTemplateActionsEnableCreatesAndDeDupeRules(t *testing.T) {
	deps := newTestAlertingDeps(t)
	deps.WebServiceHealthLogSource = collectors.WebServiceHealthLogSource

	requestBody := []byte(`{"created_by":"template-test"}`)
	req := httptest.NewRequest(
		http.MethodPost,
		"/alerts/templates/mobile.reconnect_storm/enable",
		bytes.NewReader(requestBody),
	)
	rec := httptest.NewRecorder()

	deps.HandleAlertTemplateActions(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 on first enable, got %d", rec.Code)
	}

	var created struct {
		TemplateID     string      `json:"template_id"`
		AlreadyEnabled bool        `json:"already_enabled"`
		Rule           alerts.Rule `json:"rule"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode template enable response: %v", err)
	}
	if created.TemplateID != "mobile.reconnect_storm" {
		t.Fatalf("expected template_id mobile.reconnect_storm, got %q", created.TemplateID)
	}
	if created.AlreadyEnabled {
		t.Fatalf("expected first enable to create new rule")
	}
	if created.Rule.ID == "" {
		t.Fatalf("expected created alert rule ID")
	}
	if created.Rule.Kind != alerts.RuleKindLogPattern {
		t.Fatalf("expected log_pattern rule kind, got %q", created.Rule.Kind)
	}
	if created.Rule.Metadata["template_id"] != "mobile.reconnect_storm" {
		t.Fatalf("expected metadata template_id, got %q", created.Rule.Metadata["template_id"])
	}

	// Second call should reuse the existing template-backed rule.
	secondReq := httptest.NewRequest(http.MethodPost, "/alerts/templates/mobile.reconnect_storm/enable", nil)
	secondRec := httptest.NewRecorder()

	deps.HandleAlertTemplateActions(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on second enable, got %d", secondRec.Code)
	}

	var existing struct {
		TemplateID     string      `json:"template_id"`
		AlreadyEnabled bool        `json:"already_enabled"`
		Rule           alerts.Rule `json:"rule"`
	}
	if err := json.Unmarshal(secondRec.Body.Bytes(), &existing); err != nil {
		t.Fatalf("failed to decode second template enable response: %v", err)
	}
	if !existing.AlreadyEnabled {
		t.Fatalf("expected second enable call to report already_enabled")
	}
	if existing.Rule.ID != created.Rule.ID {
		t.Fatalf("expected same template-backed rule ID, got %q and %q", created.Rule.ID, existing.Rule.ID)
	}
}

func TestHandleAlertTemplateActionsEnableServiceTemplate(t *testing.T) {
	deps := newTestAlertingDeps(t)
	deps.WebServiceHealthLogSource = collectors.WebServiceHealthLogSource

	req := httptest.NewRequest(http.MethodPost, "/alerts/templates/services.web.uptime_drop/enable", nil)
	rec := httptest.NewRecorder()

	deps.HandleAlertTemplateActions(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 on service template enable, got %d", rec.Code)
	}

	var payload struct {
		TemplateID     string      `json:"template_id"`
		AlreadyEnabled bool        `json:"already_enabled"`
		Rule           alerts.Rule `json:"rule"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode service template enable response: %v", err)
	}
	if payload.TemplateID != "services.web.uptime_drop" {
		t.Fatalf("template_id=%q want=%q", payload.TemplateID, "services.web.uptime_drop")
	}
	if payload.AlreadyEnabled {
		t.Fatalf("expected first service template enable to create a rule")
	}
	if payload.Rule.Metadata["category"] != "service_health" {
		t.Fatalf("expected service_health category metadata, got %q", payload.Rule.Metadata["category"])
	}
	if payload.Rule.Condition["source"] != collectors.WebServiceHealthLogSource {
		t.Fatalf("expected source condition %q", collectors.WebServiceHealthLogSource)
	}
}

func TestHandleAlertTemplateActionsEnableStarterTemplateForAssetTargets(t *testing.T) {
	deps := newTestAlertingDeps(t)
	deps.WebServiceHealthLogSource = collectors.WebServiceHealthLogSource

	if _, err := deps.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "asset-alpha",
		Type:    "server",
		Source:  "test",
		Name:    "Asset Alpha",
		Status:  "online",
	}); err != nil {
		t.Fatalf("failed to seed asset-alpha: %v", err)
	}
	if _, err := deps.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "asset-bravo",
		Type:    "server",
		Source:  "test",
		Name:    "Asset Bravo",
		Status:  "online",
	}); err != nil {
		t.Fatalf("failed to seed asset-bravo: %v", err)
	}

	firstReq := httptest.NewRequest(
		http.MethodPost,
		"/alerts/templates/starter.agent_offline/enable",
		bytes.NewReader([]byte(`{"targets":[{"asset_id":"asset-alpha"}]}`)),
	)
	firstRec := httptest.NewRecorder()
	deps.HandleAlertTemplateActions(firstRec, firstReq)
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 enabling asset starter template, got %d", firstRec.Code)
	}

	secondReq := httptest.NewRequest(
		http.MethodPost,
		"/alerts/templates/starter.agent_offline/enable",
		bytes.NewReader([]byte(`{"targets":[{"asset_id":"asset-bravo"}]}`)),
	)
	secondRec := httptest.NewRecorder()
	deps.HandleAlertTemplateActions(secondRec, secondReq)
	if secondRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 enabling asset starter template for second asset, got %d", secondRec.Code)
	}

	repeatReq := httptest.NewRequest(
		http.MethodPost,
		"/alerts/templates/starter.agent_offline/enable",
		bytes.NewReader([]byte(`{"targets":[{"asset_id":"asset-alpha"}]}`)),
	)
	repeatRec := httptest.NewRecorder()
	deps.HandleAlertTemplateActions(repeatRec, repeatReq)
	if repeatRec.Code != http.StatusOK {
		t.Fatalf("expected 200 when re-enabling same template/asset target, got %d", repeatRec.Code)
	}

	var firstPayload struct {
		Rule alerts.Rule `json:"rule"`
	}
	if err := json.Unmarshal(firstRec.Body.Bytes(), &firstPayload); err != nil {
		t.Fatalf("failed to decode first starter template response: %v", err)
	}
	var secondPayload struct {
		Rule alerts.Rule `json:"rule"`
	}
	if err := json.Unmarshal(secondRec.Body.Bytes(), &secondPayload); err != nil {
		t.Fatalf("failed to decode second starter template response: %v", err)
	}
	var repeatPayload struct {
		AlreadyEnabled bool        `json:"already_enabled"`
		Rule           alerts.Rule `json:"rule"`
	}
	if err := json.Unmarshal(repeatRec.Body.Bytes(), &repeatPayload); err != nil {
		t.Fatalf("failed to decode repeated starter template response: %v", err)
	}

	if firstPayload.Rule.ID == secondPayload.Rule.ID {
		t.Fatalf("expected different rule IDs for different target assets")
	}
	if !repeatPayload.AlreadyEnabled {
		t.Fatalf("expected repeat enable for same target to report already_enabled")
	}
	if repeatPayload.Rule.ID != firstPayload.Rule.ID {
		t.Fatalf("expected repeat enable to reuse first asset rule, got %q want %q", repeatPayload.Rule.ID, firstPayload.Rule.ID)
	}
	if len(firstPayload.Rule.Targets) != 1 || firstPayload.Rule.Targets[0].AssetID != "asset-alpha" {
		t.Fatalf("expected first rule target asset-alpha, got %+v", firstPayload.Rule.Targets)
	}
	if len(secondPayload.Rule.Targets) != 1 || secondPayload.Rule.Targets[0].AssetID != "asset-bravo" {
		t.Fatalf("expected second rule target asset-bravo, got %+v", secondPayload.Rule.Targets)
	}
}
