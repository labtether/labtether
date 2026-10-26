package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/synthetic"
)

func TestSyntheticFailureCountContractThroughHandlersAndEvaluation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   any
		present bool
		count   int
	}{
		{"missing defaults to three", nil, false, 3},
		{"integral numeric string", " 3.0 ", true, 3},
		{"minimum one", 1, true, 1},
		{"PostgreSQL history ceiling", 500, true, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, store, checkID := syntheticCountDeps(t)
			condition := map[string]any{"check_id": checkID}
			if tc.present {
				condition["consecutive_failures"] = tc.value
			}
			rule := createSyntheticCountRule(t, d, condition)
			evaluateSyntheticCountRule(t, d, rule, alerts.EvaluationStatusOK, false)
			for i := 0; i < tc.count-1; i++ {
				recordSyntheticCountFailure(t, store, checkID)
			}
			if tc.count > 1 {
				evaluateSyntheticCountRule(t, d, rule, alerts.EvaluationStatusOK, false)
			}
			recordSyntheticCountFailure(t, store, checkID)
			evaluateSyntheticCountRule(t, d, rule, alerts.EvaluationStatusTriggered, true)
		})
	}

	for _, tc := range []struct {
		name  string
		value any
	}{
		{"fraction below one", 0.5}, {"fraction above one", 1.9},
		{"zero", 0}, {"negative", -1}, {"over history ceiling", 501},
		{"out of integer range", 1e100}, {"null", nil}, {"boolean", true},
		{"not numeric", "many"}, {"not finite", "NaN"}, {"infinite", "+Inf"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, store, checkID := syntheticCountDeps(t)
			valid := map[string]any{"check_id": checkID, "consecutive_failures": 1}
			rule := createSyntheticCountRule(t, d, valid)
			invalid := map[string]any{"check_id": checkID, "consecutive_failures": tc.value}
			created := syntheticCountRequest(t, d, http.MethodPost, "/alerts/rules", syntheticCountPayload(invalid))
			updated := syntheticCountRequest(t, d, http.MethodPatch, "/alerts/rules/"+rule.ID, map[string]any{"condition": invalid})
			for _, reply := range []*httptest.ResponseRecorder{created, updated} {
				if reply.Code != http.StatusBadRequest || !strings.Contains(reply.Body.String(), "consecutive_failures") {
					t.Fatalf("malformed count not rejected: %d %s", reply.Code, reply.Body.String())
				}
			}
			rows, err := d.AlertStore.ListAlertRules(persistence.AlertRuleFilter{Limit: 10})
			stored, found, readErr := d.AlertStore.GetAlertRule(rule.ID)
			if err != nil || len(rows) != 1 || readErr != nil || !found || !reflect.DeepEqual(stored.Condition, rule.Condition) {
				t.Fatalf("invalid write changed stored rules: rows=%d err=%v read=%v", len(rows), err, readErr)
			}

			// Old persisted malformed rules must not fire on an empty result set.
			badRule, err := d.AlertStore.UpdateAlertRule(rule.ID, alerts.UpdateRuleRequest{Condition: &invalid})
			if err != nil {
				t.Fatal(err)
			}
			evaluateSyntheticCountRule(t, d, badRule, alerts.EvaluationStatusError, false)
			// Restore via the real update route, establish a genuine firing instance,
			// then prove the same malformed persisted condition cannot recover it.
			restored := syntheticCountRequest(t, d, http.MethodPatch, "/alerts/rules/"+rule.ID, map[string]any{"condition": valid})
			if restored.Code != http.StatusOK {
				t.Fatalf("valid restore: %d %s", restored.Code, restored.Body.String())
			}
			rule, found, err = d.AlertStore.GetAlertRule(rule.ID)
			if err != nil || !found {
				t.Fatalf("restored rule missing: %v", err)
			}
			recordSyntheticCountFailure(t, store, checkID)
			evaluateSyntheticCountRule(t, d, rule, alerts.EvaluationStatusTriggered, true)
			badRule, err = d.AlertStore.UpdateAlertRule(rule.ID, alerts.UpdateRuleRequest{Condition: &invalid})
			if err != nil {
				t.Fatal(err)
			}
			evaluateSyntheticCountRule(t, d, badRule, alerts.EvaluationStatusError, true)

			if tc.name == "fraction below one" {
				for _, unavailable := range []string{"missing check ID", "unknown check", "missing store"} {
					t.Run(unavailable, func(t *testing.T) {
						condition := map[string]any{"check_id": checkID, "consecutive_failures": tc.value}
						switch unavailable {
						case "missing check ID":
							delete(condition, "check_id")
						case "unknown check":
							condition["check_id"] = "not-a-persisted-check"
						case "missing store":
							d.SyntheticStore = nil
							defer func() { d.SyntheticStore = store }()
						}
						persisted, err := d.AlertStore.UpdateAlertRule(rule.ID, alerts.UpdateRuleRequest{Condition: &condition})
						if err != nil {
							t.Fatal(err)
						}
						evaluateSyntheticCountRule(t, d, persisted, alerts.EvaluationStatusError, true)
					})
				}
			}
		})
	}
}

func syntheticCountDeps(t *testing.T) (*Deps, *persistence.MemorySyntheticStore, string) {
	t.Helper()
	d := newTestAlertingDeps(t)
	store := persistence.NewMemorySyntheticStore()
	d.SyntheticStore = store
	t.Cleanup(d.WaitForNotificationDispatches)
	check, err := store.CreateSyntheticCheck(synthetic.CreateCheckRequest{Name: "Failure count", CheckType: "http", Target: "https://example.invalid/health"})
	if err != nil {
		t.Fatal(err)
	}
	return d, store, check.ID
}

func syntheticCountPayload(condition map[string]any) map[string]any {
	return map[string]any{"name": "Synthetic count", "kind": alerts.RuleKindSyntheticCheck,
		"severity": "high", "target_scope": "global", "condition": condition}
}

func syntheticCountRequest(t *testing.T, d *Deps, method, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	rec := httptest.NewRecorder()
	if path == "/alerts/rules" {
		d.HandleAlertRules(rec, req)
	} else {
		d.HandleAlertRuleActions(rec, req)
	}
	return rec
}

func createSyntheticCountRule(t *testing.T, d *Deps, condition map[string]any) alerts.Rule {
	t.Helper()
	rec := syntheticCountRequest(t, d, http.MethodPost, "/alerts/rules", syntheticCountPayload(condition))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var reply struct {
		Rule alerts.Rule `json:"rule"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil || reply.Rule.ID == "" {
		t.Fatalf("create response: %v", err)
	}
	return reply.Rule
}

func recordSyntheticCountFailure(t *testing.T, store *persistence.MemorySyntheticStore, checkID string) {
	t.Helper()
	if _, err := store.RecordSyntheticResult(checkID, synthetic.Result{Status: synthetic.ResultStatusFail}); err != nil {
		t.Fatal(err)
	}
}

func evaluateSyntheticCountRule(t *testing.T, d *Deps, rule alerts.Rule, want string, active bool) {
	t.Helper()
	before, err := d.AlertStore.ListAlertEvaluations(rule.ID, 20)
	if err != nil || len(before) >= 20 {
		t.Fatalf("evaluation baseline: %d %v", len(before), err)
	}
	seen := map[string]bool{}
	for _, row := range before {
		seen[row.ID] = true
	}
	fingerprint := alerts.GenerateFingerprint(rule.ID, rule.Labels)
	oldInstance, oldFound, err := d.AlertInstanceStore.GetActiveInstanceByFingerprint(rule.ID, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	d.EvaluateSingleRule(context.Background(), rule, nil)
	d.WaitForNotificationDispatches()
	rows, err := d.AlertStore.ListAlertEvaluations(rule.ID, 20)
	if err != nil || len(rows) != len(before)+1 {
		t.Fatalf("new evaluation missing: before=%d after=%d err=%v", len(before), len(rows), err)
	}
	var added []alerts.Evaluation
	for _, row := range rows {
		if !seen[row.ID] {
			added = append(added, row)
		}
	}
	if len(added) != 1 || added[0].Status != want || (want == alerts.EvaluationStatusError && !strings.Contains(added[0].Error, "consecutive_failures")) {
		t.Fatalf("evaluation %s: %+v", want, added)
	}
	instance, found, err := d.AlertInstanceStore.GetActiveInstanceByFingerprint(rule.ID, fingerprint)
	if err != nil || found != active || (found && instance.Status != alerts.InstanceStatusFiring) {
		t.Fatalf("active instance: found=%v status=%s err=%v", found, instance.Status, err)
	}
	if want == alerts.EvaluationStatusError && (oldFound != found || !reflect.DeepEqual(oldInstance, instance)) {
		t.Fatal("invalid saved count changed the active instance")
	}
}
