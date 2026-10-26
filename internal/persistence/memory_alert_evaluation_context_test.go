package persistence

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/telemetry"
)

// These are actual store locks. A context check followed by ordinary RLock or
// Lock fails: the writer remains held until after the canceled call returns.
func TestMemoryAlertEvaluationCancelsLocksAndRejectsPartialMetrics(t *testing.T) {
	assets := NewMemoryAssetStore()
	groups := NewMemoryGroupStore()
	capabilities := NewMemoryCanonicalModelStore()
	metrics := NewMemoryTelemetryStore()
	events := NewMemoryLogStore()
	checks := NewMemorySyntheticStore()
	rules := NewMemoryAlertStore()
	instances := NewMemoryAlertInstanceStore()
	incidentStore := NewMemoryIncidentStore()
	now := time.Now().UTC()
	cases := []struct {
		name string
		mu   *sync.RWMutex
		call func(context.Context) error
	}{
		{"authorization groups", &groups.mu, func(ctx context.Context) error { _, e := groups.ListGroupsContext(ctx); return e }},
		{"inventory", &assets.mu, func(ctx context.Context) error { _, e := assets.ListAssetsContext(ctx); return e }},
		{"capabilities", &capabilities.mu, func(ctx context.Context) error { _, e := capabilities.ListCapabilitySetsContext(ctx, 500); return e }},
		{"metric rows", &metrics.mu, func(ctx context.Context) error {
			_, e := metrics.MetricSeriesBatchContext(ctx, []string{"node"}, "cpu_used_percent", now.Add(-time.Minute), now, 0, 3)
			return e
		}},
		{"metric activity", &metrics.mu, func(ctx context.Context) error {
			_, e := metrics.AssetsWithMetricSamplesContext(ctx, []string{"node"}, "cpu_used_percent", now.Add(-time.Minute), now)
			return e
		}},
		{"log rows", &events.mu, func(ctx context.Context) error { _, e := events.QueryEventsContext(ctx, logs.QueryRequest{}); return e }},
		{"synthetic rows", &checks.mu, func(ctx context.Context) error { _, e := checks.ListSyntheticResultsContext(ctx, "check", 3); return e }},
		{"manual exact rule", &rules.mu, func(ctx context.Context) error { _, _, e := rules.GetAlertRuleContext(ctx, "rule"); return e }},
		{"automatic claim", &rules.mu, func(ctx context.Context) error { _, e := rules.ClaimAlertRuleEvaluation(ctx, "rule", now); return e }},
		{"evaluation history", &rules.mu, func(ctx context.Context) error {
			_, e := rules.RecordAlertEvaluationContext(ctx, "rule", alerts.Evaluation{})
			return e
		}},
		{"instance create", &instances.mu, func(ctx context.Context) error {
			_, e := instances.CreateAlertInstanceContext(ctx, alerts.CreateInstanceRequest{RuleID: "rule"})
			return e
		}},
		{"instance resolve", &instances.mu, func(ctx context.Context) error {
			_, e := instances.UpdateAlertInstanceStatusContext(ctx, "instance", alerts.InstanceStatusResolved)
			return e
		}},
		{"instance refire", &instances.mu, func(ctx context.Context) error { return instances.UpdateAlertInstanceLastFiredContext(ctx, "instance") }},
		{"first fire", &instances.mu, func(ctx context.Context) error {
			_, e := instances.FireAlertInstance(ctx, "instance", alerts.FiringScope{Version: 1})
			return e
		}},
		{"firing history", &instances.mu, func(ctx context.Context) error {
			_, e := instances.AlertFingerprintTiming(ctx, "rule", "fingerprint")
			return e
		}},
		{"composite snapshot", &instances.mu, func(ctx context.Context) error {
			return instances.ReadFiringAlertInstances(ctx, []string{"rule"}, func(alerts.AlertInstance) error { return nil })
		}},
		{"silence read", &instances.mu, func(ctx context.Context) error { _, e := instances.ListAlertSilencesContext(ctx, 500, true); return e }},
		{"auto incident create", &incidentStore.mu, func(ctx context.Context) error {
			_, e := incidentStore.CreateIncidentContext(ctx, incidents.CreateIncidentRequest{Title: "cancel"})
			return e
		}},
		{"auto incident lookup", &incidentStore.mu, func(ctx context.Context) error {
			_, e := incidentStore.HasAutoIncidentForAlertInstanceContext(ctx, "instance")
			return e
		}},
		{"auto incident link", &incidentStore.mu, func(ctx context.Context) error {
			_, e := incidentStore.LinkIncidentAlertContext(ctx, "incident", incidents.LinkAlertRequest{AlertInstanceID: "instance", LinkType: incidents.LinkTypeTrigger})
			return e
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			observed := &observedRulePageContext{Context: ctx, firstCheck: make(chan struct{})}
			tc.mu.Lock()
			done := make(chan error, 1)
			go func() { done <- tc.call(observed) }()
			select {
			case <-observed.firstCheck:
			case <-time.After(time.Second):
				cancel()
				tc.mu.Unlock()
				<-done
				t.Fatal("context never reached store")
			}
			cancel()
			select {
			case err := <-done:
				tc.mu.Unlock()
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancel result: %v", err)
				}
			case <-time.After(time.Second):
				tc.mu.Unlock()
				<-done
				t.Fatal("canceled store waited for writer")
			}
		})
	}
	if len(instances.instances) != 0 || len(incidentStore.incidents) != 0 {
		t.Fatal("canceled writes created rows")
	}
	samples := []telemetry.MetricSample{}
	for _, value := range []float64{10, 20, 90} {
		samples = append(samples, telemetry.MetricSample{AssetID: "node", Metric: "cpu_used_percent", Unit: "percent", Value: value, CollectedAt: now})
	}
	samples = append(samples, telemetry.MetricSample{AssetID: "node", Metric: "cpu_used_percent", Unit: "percent", Value: 999, CollectedAt: now.Add(-time.Hour)})
	if err := metrics.AppendSamples(context.Background(), samples); err != nil {
		t.Fatal(err)
	}
	rows, err := metrics.MetricSeriesBatchContext(context.Background(), []string{"node"}, "cpu_used_percent", now.Add(-time.Minute), now, 0, 2)
	if !errors.Is(err, ErrTelemetryQueryLimitExceeded) || rows != nil {
		t.Fatalf("overflow returned partial rows: %+v %v", rows, err)
	}
	rows, err = metrics.MetricSeriesBatchContext(context.Background(), []string{"node"}, "cpu_used_percent", now.Add(-time.Minute), now, 0, 3)
	if err != nil || len(rows["node"].Points) != 3 {
		t.Fatalf("exact limit lost data: %+v %v", rows, err)
	}
}
