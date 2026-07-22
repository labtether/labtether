package proxmox

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
)

type actionRoutingCollectorStore struct {
	collectors []hubcollector.Collector
	listErr    error
}

func (s *actionRoutingCollectorStore) CreateHubCollector(hubcollector.CreateCollectorRequest) (hubcollector.Collector, error) {
	return hubcollector.Collector{}, fmt.Errorf("not implemented")
}
func (s *actionRoutingCollectorStore) GetHubCollector(id string) (hubcollector.Collector, bool, error) {
	for _, collector := range s.collectors {
		if collector.ID == id {
			return collector, true, nil
		}
	}
	return hubcollector.Collector{}, false, nil
}
func (s *actionRoutingCollectorStore) ListHubCollectors(int, bool) ([]hubcollector.Collector, error) {
	return append([]hubcollector.Collector(nil), s.collectors...), s.listErr
}
func (s *actionRoutingCollectorStore) UpdateHubCollector(string, hubcollector.UpdateCollectorRequest) (hubcollector.Collector, error) {
	return hubcollector.Collector{}, fmt.Errorf("not implemented")
}
func (s *actionRoutingCollectorStore) DeleteHubCollector(string) error {
	return fmt.Errorf("not implemented")
}
func (s *actionRoutingCollectorStore) UpdateHubCollectorStatus(string, string, string, time.Time) error {
	return nil
}

func TestExecuteActionInProcessFailsClosedForMultipleCollectorRuntimes(t *testing.T) {
	t.Parallel()
	assetStore := persistence.NewMemoryAssetStore()
	target := assetid.ScopeCollectorAssetID("portainer-container-2-abc123", "collector-portainer-a")
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: target,
		Type:    "container",
		Name:    "test",
		Source:  "portainer",
		Metadata: map[string]string{
			"collector_id": "collector-portainer-a",
		},
	}); err != nil {
		t.Fatal(err)
	}

	fallbackCalls := 0
	d := &Deps{
		AssetStore: assetStore,
		HubCollectorStore: &actionRoutingCollectorStore{collectors: []hubcollector.Collector{
			{ID: "collector-portainer-a", CollectorType: hubcollector.CollectorTypePortainer, Enabled: true},
			{ID: "collector-portainer-b", CollectorType: hubcollector.CollectorTypePortainer, Enabled: true},
		}},
		ExecuteActionInProcessFn: func(job actions.Job, registry *connectorsdk.Registry) actions.Result {
			fallbackCalls++
			return actions.Result{Status: actions.StatusSucceeded}
		},
	}
	result := d.ExecuteActionInProcess(actions.Job{
		JobID:       "job-1",
		RunID:       "run-1",
		Type:        actions.RunTypeConnectorAction,
		ConnectorID: "portainer",
		ActionID:    "container.restart",
		Target:      target,
	})
	if result.Status != actions.StatusFailed || !strings.Contains(result.Error, "multiple active portainer collectors") {
		t.Fatalf("expected fail-closed result, got %+v", result)
	}
	if fallbackCalls != 0 {
		t.Fatalf("ambiguous action reached singleton connector fallback %d times", fallbackCalls)
	}
}

func TestExecuteActionInProcessAllowsUnambiguousCollectorRuntime(t *testing.T) {
	t.Parallel()
	assetStore := persistence.NewMemoryAssetStore()
	target := assetid.ScopeCollectorAssetID("portainer-container-2-abc123", "collector-portainer-a")
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: target,
		Type:    "container",
		Name:    "test",
		Source:  "portainer",
		Metadata: map[string]string{
			"collector_id": "collector-portainer-a",
		},
	}); err != nil {
		t.Fatal(err)
	}

	fallbackCalls := 0
	d := &Deps{
		AssetStore: assetStore,
		HubCollectorStore: &actionRoutingCollectorStore{collectors: []hubcollector.Collector{
			{ID: "collector-portainer-a", CollectorType: hubcollector.CollectorTypePortainer, Enabled: true},
		}},
		ExecuteActionInProcessFn: func(job actions.Job, registry *connectorsdk.Registry) actions.Result {
			fallbackCalls++
			return actions.Result{JobID: job.JobID, RunID: job.RunID, Status: actions.StatusSucceeded}
		},
	}
	result := d.ExecuteActionInProcess(actions.Job{
		JobID:       "job-2",
		RunID:       "run-2",
		Type:        actions.RunTypeConnectorAction,
		ConnectorID: "portainer",
		ActionID:    "container.restart",
		Target:      target,
	})
	if result.Status != actions.StatusSucceeded || fallbackCalls != 1 {
		t.Fatalf("expected unambiguous fallback once, result=%+v calls=%d", result, fallbackCalls)
	}
}

func TestExecuteActionInProcessFailsClosedWhenScopedOwnershipCannotBeVerified(t *testing.T) {
	t.Parallel()
	d := &Deps{
		HubCollectorStore: &actionRoutingCollectorStore{listErr: fmt.Errorf("database unavailable")},
		ExecuteActionInProcessFn: func(job actions.Job, registry *connectorsdk.Registry) actions.Result {
			t.Fatal("fallback must not run")
			return actions.Result{}
		},
	}
	result := d.ExecuteActionInProcess(actions.Job{
		JobID:       "job-3",
		RunID:       "run-3",
		Type:        actions.RunTypeConnectorAction,
		ConnectorID: "home-assistant",
		ActionID:    "entity.toggle",
		Target:      assetid.ScopeCollectorAssetID("ha-entity-sun-sun", "collector-ha-a"),
	})
	if result.Status != actions.StatusFailed || !strings.Contains(result.Error, "cannot verify collector-aware action routing") {
		t.Fatalf("expected ownership verification failure, got %+v", result)
	}
}
