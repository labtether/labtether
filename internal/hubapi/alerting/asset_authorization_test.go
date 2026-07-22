package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/dependencies"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/persistence"
)

type authorizationDependencyStore struct {
	incidentAssets map[string][]incidents.IncidentAsset
}

func (s *authorizationDependencyStore) CreateAssetDependency(req dependencies.CreateDependencyRequest) (dependencies.Dependency, error) {
	return dependencies.Dependency{SourceAssetID: req.SourceAssetID, TargetAssetID: req.TargetAssetID}, nil
}
func (s *authorizationDependencyStore) ListAssetDependencies(string, int) ([]dependencies.Dependency, error) {
	return nil, nil
}
func (s *authorizationDependencyStore) GetAssetDependency(string) (dependencies.Dependency, bool, error) {
	return dependencies.Dependency{}, false, nil
}
func (s *authorizationDependencyStore) DeleteAssetDependency(string) error { return nil }
func (s *authorizationDependencyStore) BlastRadius(string, int) ([]dependencies.ImpactNode, error) {
	return nil, nil
}
func (s *authorizationDependencyStore) UpstreamCauses(string, int) ([]dependencies.ImpactNode, error) {
	return nil, nil
}
func (s *authorizationDependencyStore) LinkIncidentAsset(incidentID string, req incidents.LinkAssetRequest) (incidents.IncidentAsset, error) {
	link := incidents.IncidentAsset{ID: "link-" + req.AssetID, IncidentID: incidentID, AssetID: req.AssetID, Role: req.Role}
	s.incidentAssets[incidentID] = append(s.incidentAssets[incidentID], link)
	return link, nil
}
func (s *authorizationDependencyStore) ListIncidentAssets(incidentID string, _ int) ([]incidents.IncidentAsset, error) {
	return append([]incidents.IncidentAsset(nil), s.incidentAssets[incidentID]...), nil
}
func (s *authorizationDependencyStore) UnlinkIncidentAsset(incidentID, linkID string) error {
	links := s.incidentAssets[incidentID]
	for i, link := range links {
		if link.ID == linkID {
			s.incidentAssets[incidentID] = append(links[:i], links[i+1:]...)
			return nil
		}
	}
	return persistence.ErrNotFound
}

func restrictedAlertRequest(method, path string, body []byte, allowed ...string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	return req.WithContext(apiv2.ContextWithAllowedAssets(context.Background(), allowed))
}

func TestRestrictedAlertRuleAndInstanceCollectionsDoNotLeakSecretTargets(t *testing.T) {
	d := newTestAlertingDeps(t)
	allowedRule, err := d.AlertStore.CreateAlertRule(alerts.CreateRuleRequest{
		Name: "allowed", Targets: []alerts.RuleTargetInput{{AssetID: "asset-a"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	secretRule, err := d.AlertStore.CreateAlertRule(alerts.CreateRuleRequest{
		Name: "secret", Targets: []alerts.RuleTargetInput{{AssetID: "asset-b"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.AlertStore.CreateAlertRule(alerts.CreateRuleRequest{
		Name: "global", Targets: []alerts.RuleTargetInput{{Selector: map[string]any{"platform": "linux"}}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.AlertInstanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{RuleID: allowedRule.ID, Severity: alerts.SeverityHigh}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.AlertInstanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{RuleID: secretRule.ID, Severity: alerts.SeverityHigh}); err != nil {
		t.Fatal(err)
	}

	rulesRec := httptest.NewRecorder()
	d.HandleAlertRules(rulesRec, restrictedAlertRequest(http.MethodGet, "/alerts/rules", nil, "asset-a"))
	if rulesRec.Code != http.StatusOK {
		t.Fatalf("rules: expected 200, got %d body=%s", rulesRec.Code, rulesRec.Body.String())
	}
	var rulesResponse struct {
		Rules []alerts.Rule `json:"rules"`
	}
	if err := json.Unmarshal(rulesRec.Body.Bytes(), &rulesResponse); err != nil {
		t.Fatal(err)
	}
	if len(rulesResponse.Rules) != 1 || rulesResponse.Rules[0].ID != allowedRule.ID {
		t.Fatalf("unexpected filtered rules: %#v", rulesResponse.Rules)
	}

	instancesRec := httptest.NewRecorder()
	d.HandleAlertInstances(instancesRec, restrictedAlertRequest(http.MethodGet, "/alerts/instances", nil, "asset-a"))
	if instancesRec.Code != http.StatusOK {
		t.Fatalf("instances: expected 200, got %d body=%s", instancesRec.Code, instancesRec.Body.String())
	}
	var instancesResponse struct {
		Instances []alerts.AlertInstance `json:"instances"`
	}
	if err := json.Unmarshal(instancesRec.Body.Bytes(), &instancesResponse); err != nil {
		t.Fatal(err)
	}
	if len(instancesResponse.Instances) != 1 || instancesResponse.Instances[0].RuleID != allowedRule.ID {
		t.Fatalf("unexpected filtered instances: %#v", instancesResponse.Instances)
	}

	secretRec := httptest.NewRecorder()
	d.HandleAlertRuleActions(secretRec, restrictedAlertRequest(http.MethodGet, "/alerts/rules/"+secretRule.ID, nil, "asset-a"))
	if secretRec.Code != http.StatusForbidden {
		t.Fatalf("secret rule: expected 403, got %d body=%s", secretRec.Code, secretRec.Body.String())
	}
}

func TestRestrictedAlertSilencesFailClosed(t *testing.T) {
	d := newTestAlertingDeps(t)
	rec := httptest.NewRecorder()
	d.HandleAlertSilences(rec, restrictedAlertRequest(http.MethodGet, "/alerts/silences", nil, "asset-a"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRestrictedIncidentCollectionAndWritesEnforcePrimaryAsset(t *testing.T) {
	d := newTestAlertingDeps(t)
	d.DependencyStore = &authorizationDependencyStore{incidentAssets: map[string][]incidents.IncidentAsset{}}
	testutil.CreateTestAsset(t, d.AssetStore, "asset-a", "server", "Allowed")
	testutil.CreateTestAsset(t, d.AssetStore, "asset-b", "server", "Secret")
	allowedIncident, err := d.IncidentStore.CreateIncident(incidents.CreateIncidentRequest{Title: "allowed", PrimaryAssetID: "asset-a"})
	if err != nil {
		t.Fatal(err)
	}
	secretIncident, err := d.IncidentStore.CreateIncident(incidents.CreateIncidentRequest{Title: "secret", PrimaryAssetID: "asset-b"})
	if err != nil {
		t.Fatal(err)
	}

	listRec := httptest.NewRecorder()
	d.HandleIncidents(listRec, restrictedAlertRequest(http.MethodGet, "/incidents", nil, "asset-a"))
	if listRec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var response struct {
		Incidents []incidents.Incident `json:"incidents"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Incidents) != 1 || response.Incidents[0].ID != allowedIncident.ID {
		t.Fatalf("unexpected filtered incidents: %#v", response.Incidents)
	}

	readRec := httptest.NewRecorder()
	d.HandleIncidentActions(readRec, restrictedAlertRequest(http.MethodGet, "/incidents/"+secretIncident.ID, nil, "asset-a"))
	if readRec.Code != http.StatusForbidden {
		t.Fatalf("read: expected 403, got %d body=%s", readRec.Code, readRec.Body.String())
	}

	createRec := httptest.NewRecorder()
	d.HandleIncidents(createRec, restrictedAlertRequest(
		http.MethodPost,
		"/incidents",
		[]byte(`{"title":"forbidden","severity":"high","primary_asset_id":"asset-b"}`),
		"asset-a",
	))
	if createRec.Code != http.StatusForbidden {
		t.Fatalf("create: expected 403, got %d body=%s", createRec.Code, createRec.Body.String())
	}
}

var _ persistence.DependencyStore = (*authorizationDependencyStore)(nil)
