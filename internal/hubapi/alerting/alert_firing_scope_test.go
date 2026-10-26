package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/synthetic"
	"github.com/labtether/labtether/internal/telemetry"
)

type scopeTelemetryRead struct {
	*persistence.MemoryTelemetryStore
	reads int
}

func (s *scopeTelemetryRead) MetricSeriesBatchContext(ctx context.Context, ids []string, metric string, from, to time.Time, step time.Duration, maxRawPoints int) (map[string]telemetry.Series, error) {
	s.reads++
	if s.reads > 1 {
		return nil, errors.New("condition was re-read to infer scope")
	}
	return s.MemoryTelemetryStore.MetricSeriesBatchContext(ctx, ids, metric, from, to, step, maxRawPoints)
}

type scopeAssetLookup struct {
	persistence.AssetStore
	legacy, reads int
	deadlines     []time.Time
	stop          context.CancelFunc
}

func (s *scopeAssetLookup) GetAsset(string) (assets.Asset, bool, error) {
	s.legacy++
	return assets.Asset{}, false, errors.New("legacy background asset lookup used")
}
func (s *scopeAssetLookup) GetAssetContext(ctx context.Context, _ string) (assets.Asset, bool, error) {
	s.reads++
	deadline, ok := ctx.Deadline()
	if !ok {
		return assets.Asset{}, false, errors.New("missing lookup bound")
	}
	s.deadlines = append(s.deadlines, deadline)
	if s.stop != nil {
		s.stop()
	}
	return assets.Asset{}, false, errors.New("optional group lookup unavailable")
}

func firingScopeDeps(t *testing.T) (*Deps, *persistence.MemoryAssetStore) {
	t.Helper()
	d := newTestAlertingDeps(t)
	raw := persistence.NewMemoryAssetStore()
	d.AssetStore = raw
	for _, id := range []string{"scope-a", "scope-b"} {
		if _, err := raw.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: id, Type: "host", Source: "agent", Status: "online", GroupID: "group-" + id}); err != nil {
			t.Fatal(err)
		}
	}
	return d, raw
}
func scopeRule(t *testing.T, d *Deps, kind string, condition map[string]any, global bool) alerts.Rule {
	t.Helper()
	targets := []alerts.RuleTargetInput{{AssetID: "scope-a"}, {AssetID: "scope-b"}}
	scope := alerts.TargetScopeAsset
	if global {
		targets = nil
		scope = alerts.TargetScopeGlobal
	}
	rule, err := d.AlertStore.CreateAlertRule(alerts.CreateRuleRequest{Name: "scope-rule", Kind: kind, Severity: "high", TargetScope: scope, Targets: targets, Condition: condition})
	if err != nil {
		t.Fatal(err)
	}
	return rule
}
func evaluateScope(t *testing.T, d *Deps, rule alerts.Rule, want alerts.FiringScope) alerts.AlertInstance {
	t.Helper()
	d.EvaluateSingleRule(context.Background(), rule, nil)
	d.WaitForNotificationDispatches()
	row, ok, err := d.AlertInstanceStore.GetActiveInstanceByFingerprint(rule.ID, alerts.GenerateFingerprint(rule.ID, rule.Labels))
	normalized, normalizeErr := alerts.NormalizeFiringScope(want)
	if err != nil || !ok || row.Status != alerts.InstanceStatusFiring || row.FirstFiredAt == nil || normalizeErr != nil || row.FiringScope == nil || !reflect.DeepEqual(*row.FiringScope, normalized) {
		t.Fatalf("actual firing witness: %+v want %+v err=%v", row, want, err)
	}
	return row
}
func assetAScope() alerts.FiringScope {
	return alerts.FiringScope{Version: 1, AssetIDs: []string{"scope-a"}, GroupIDs: []string{"group-scope-a"}}
}

func TestAlertFiringScopeUsesConditionWitnesses(t *testing.T) {
	t.Run("metric same read and immutable refire", func(t *testing.T) {
		d, raw := firingScopeDeps(t)
		metric := &scopeTelemetryRead{MemoryTelemetryStore: persistence.NewMemoryTelemetryStore()}
		d.TelemetryStore = metric
		for id, value := range map[string]float64{"scope-a": 90, "scope-b": 10} {
			if err := metric.AppendSamples(context.Background(), []telemetry.MetricSample{{AssetID: id, Metric: "cpu_used_percent", Unit: "percent", Value: value, CollectedAt: time.Now().UTC()}}); err != nil {
				t.Fatal(err)
			}
		}
		rule := scopeRule(t, d, alerts.RuleKindMetricThreshold, map[string]any{"metric": "cpu_used_percent", "operator": ">", "value": 80.0}, false)
		first := evaluateScope(t, d, rule, assetAScope())
		if metric.reads != 1 {
			t.Fatal("more than one condition read")
		}
		moved := "group-moved"
		if _, err := raw.UpdateAsset("scope-a", assets.UpdateRequest{GroupID: &moved}); err != nil {
			t.Fatal(err)
		}
		rule.Targets = []alerts.RuleTarget{{AssetID: "scope-b"}}
		rule.Condition["value"] = 0.0
		metric.reads = 0
		second := evaluateScope(t, d, rule, assetAScope())
		if first.ID != second.ID || !first.FirstFiredAt.Equal(*second.FirstFiredAt) {
			t.Fatal("refire changed original instance identity")
		}
		// Exercise the actual stored snapshot through the correlator and timeline
		// handler after both the rule and current asset group have changed.
		d.IncidentEventStore = &correlatedTimelineStore{}
		for _, target := range []struct {
			asset, group string
			want         bool
		}{
			{"scope-a", "", true}, {"scope-b", "", false},
			{"", "group-scope-a", true}, {"", "group-moved", false},
		} {
			inc, err := d.IncidentStore.CreateIncident(incidents.CreateIncidentRequest{Title: "Scope witness", Severity: "high", PrimaryAssetID: target.asset, GroupID: target.group})
			if err != nil {
				t.Fatal(err)
			}
			inc.OpenedAt = first.FirstFiredAt.Add(-time.Minute)
			d.correlateIncident(inc, correlationPage{alertInstances: []alerts.AlertInstance{second}})
			rec := httptest.NewRecorder()
			d.HandleIncidentActions(rec, httptest.NewRequest(http.MethodGet, "/incidents/"+inc.ID+"/timeline", nil))
			var reply struct {
				Events []incidents.IncidentEvent `json:"events"`
			}
			if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &reply) != nil {
				t.Fatalf("timeline: %d %s", rec.Code, rec.Body.String())
			}
			count := 0
			if target.want {
				count = 1
			}
			if len(reply.Events) != count {
				t.Fatalf("timeline escaped firing owner %+v: %+v", target, reply.Events)
			}
			if count == 1 && (reply.Events[0].SourceRef != "alert_instances:"+first.ID || reply.Events[0].IncidentID != inc.ID || !reply.Events[0].OccurredAt.Equal(*first.FirstFiredAt)) {
				t.Fatalf("wrong firing timeline: %+v", reply.Events[0])
			}
		}
	})
	t.Run("heartbeat", func(t *testing.T) {
		d, raw := firingScopeDeps(t)
		raw.BackdateLastSeenAt("scope-a", time.Now().Add(-120*time.Second))
		evaluateScope(t, d, scopeRule(t, d, alerts.RuleKindHeartbeatStale, map[string]any{"max_stale_seconds": 60}, false), assetAScope())
	})
	t.Run("deadman selected metric", func(t *testing.T) {
		d, _ := firingScopeDeps(t)
		if err := d.TelemetryStore.AppendSamples(context.Background(), []telemetry.MetricSample{{AssetID: "scope-b", Metric: "cpu_used_percent", Unit: "percent", Value: 10, CollectedAt: time.Now().UTC()}}); err != nil {
			t.Fatal(err)
		}
		evaluateScope(t, d, scopeRule(t, d, alerts.RuleKindMetricDeadman, map[string]any{"metric": "cpu_used_percent", "max_silence_seconds": 60}, false), assetAScope())
	})
	for _, global := range []bool{false, true} {
		name := "targeted log"
		if global {
			name = "global log contributing rows"
		}
		t.Run(name, func(t *testing.T) {
			d, _ := firingScopeDeps(t)
			ids := []string{"scope-a", "scope-a", "scope-b"}
			if global {
				ids = []string{"scope-a", "scope-b"}
			}
			for _, id := range ids {
				if err := d.LogStore.AppendEvent(logs.Event{AssetID: id, Source: "fixture", Level: "error", Message: "failed", Timestamp: time.Now().UTC()}); err != nil {
					t.Fatal(err)
				}
			}
			want := assetAScope()
			if global {
				want.AssetIDs = append(want.AssetIDs, "scope-b")
				want.GroupIDs = append(want.GroupIDs, "group-scope-b")
			}
			evaluateScope(t, d, scopeRule(t, d, alerts.RuleKindLogPattern, map[string]any{"pattern": "failed", "min_occurrences": 2.0}, global), want)
		})
	}
	t.Run("global log group enrichment stays optional and bounded", func(t *testing.T) {
		for _, mode := range []string{"prefetch", "no-context-owner", "lookup-error", "pre-cancel", "cancel-after-first"} {
			t.Run(mode, func(t *testing.T) {
				d, raw := firingScopeDeps(t)
				for _, id := range []string{"scope-a", "scope-b"} {
					if err := d.LogStore.AppendEvent(logs.Event{AssetID: id, Source: "fixture", Level: "error", Message: "failed", Timestamp: time.Now().UTC()}); err != nil {
						t.Fatal(err)
					}
				}
				rule := scopeRule(t, d, alerts.RuleKindLogPattern, map[string]any{"pattern": "failed", "min_occurrences": 2.0}, true)
				probe := &scopeAssetLookup{AssetStore: raw}
				d.AssetStore = probe
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				var prefetch []assets.Asset
				want := alerts.FiringScope{Version: 1, AssetIDs: []string{"scope-a", "scope-b"}}
				wantReads := 0
				switch mode {
				case "prefetch":
					var err error
					prefetch, err = raw.ListAssets()
					if err != nil {
						t.Fatal(err)
					}
					want.GroupIDs = []string{"group-scope-a", "group-scope-b"}
				case "no-context-owner":
					d.AssetStore = struct{ persistence.AssetStore }{probe}
				case "lookup-error":
					wantReads = 2
				case "pre-cancel":
					cancel()
				case "cancel-after-first":
					probe.stop = cancel
					wantReads = 1
				}
				outcome, err := d.evaluateLogPatternOutcome(ctx, rule, prefetch, nil)
				if mode == "pre-cancel" || mode == "cancel-after-first" {
					if !errors.Is(err, context.Canceled) || outcome.Triggered {
						t.Fatalf("canceled condition returned witnesses: %+v %v", outcome, err)
					}
				} else if err != nil || !outcome.Triggered || !reflect.DeepEqual(outcome.Scope, want) {
					t.Fatalf("log witnesses lost: %+v/%v", outcome, err)
				}
				if probe.legacy != 0 || probe.reads != wantReads || len(probe.deadlines) != probe.reads {
					t.Fatalf("unexpected reads legacy=%d context=%d bounded=%d", probe.legacy, probe.reads, len(probe.deadlines))
				}
				for _, deadline := range probe.deadlines {
					if deadline != probe.deadlines[0] || time.Until(deadline) > 5*time.Second {
						t.Fatal("enrichment did not share one bounded deadline")
					}
				}
			})
		}
	})
	t.Run("composite saved branch scope and unknown legacy", func(t *testing.T) {
		d, _ := firingScopeDeps(t)
		seed := func(fp string, scope *alerts.FiringScope) alerts.Rule {
			rule := scopeRule(t, d, alerts.RuleKindHeartbeatStale, map[string]any{"max_stale_seconds": 60}, false)
			inst, err := d.AlertInstanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{RuleID: rule.ID, Fingerprint: fp})
			if err != nil {
				t.Fatal(err)
			}
			if scope == nil {
				_, err = d.AlertInstanceStore.UpdateAlertInstanceStatus(inst.ID, alerts.InstanceStatusFiring)
			} else {
				_, err = d.AlertInstanceStore.(persistence.AlertInstanceFiringStore).FireAlertInstance(context.Background(), inst.ID, *scope)
			}
			if err != nil {
				t.Fatal(err)
			}
			return rule
		}
		a := assetAScope()
		known := seed("known", &a)
		legacy := seed("legacy", nil)
		rule := scopeRule(t, d, alerts.RuleKindComposite, map[string]any{"rule_ids": []interface{}{known.ID, legacy.ID}, "operator": "and"}, false)
		// Both referenced rules target A+B, but only the saved A witness is known.
		evaluateScope(t, d, rule, assetAScope())
	})
	t.Run("synthetic exact service ownership or unknown", func(t *testing.T) {
		for _, known := range []bool{true, false} {
			d, _ := firingScopeDeps(t)
			d.SyntheticStore = persistence.NewMemorySyntheticStore()
			check, err := d.SyntheticStore.CreateSyntheticCheck(synthetic.CreateCheckRequest{Name: "owned", CheckType: "http", Target: "https://not-an-asset.invalid", ServiceID: "service-id"})
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 3; i++ {
				if _, err := d.SyntheticStore.RecordSyntheticResult(check.ID, synthetic.Result{Status: "fail"}); err != nil {
					t.Fatal(err)
				}
			}
			d.ResolveSyntheticServiceAsset = func(_ context.Context, id string) (assets.Asset, bool, error) {
				if id != "service-id" {
					t.Fatal("wrong service identity")
				}
				if !known {
					return assets.Asset{}, false, errors.New("ambiguous")
				}
				return assets.Asset{ID: "scope-a", GroupID: "group-scope-a"}, true, nil
			}
			want := alerts.FiringScope{Version: 1, SyntheticCheckIDs: []string{check.ID}}
			if known {
				want.AssetIDs = []string{"scope-a"}
				want.GroupIDs = []string{"group-scope-a"}
			}
			evaluateScope(t, d, scopeRule(t, d, alerts.RuleKindSyntheticCheck, map[string]any{"check_id": check.ID}, false), want)
		}
	})
}
