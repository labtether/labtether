package modelmap

import (
	"context"
	"testing"

	"github.com/labtether/labtether/internal/connectorsdk"
)

type fakeConnector struct {
	actionIDSeen string
}

func (f *fakeConnector) ID() string          { return "docker" }
func (f *fakeConnector) DisplayName() string { return "Docker" }
func (f *fakeConnector) Capabilities() connectorsdk.Capabilities {
	return connectorsdk.Capabilities{DiscoverAssets: true, ExecuteActions: true}
}
func (f *fakeConnector) Discover(context.Context) ([]connectorsdk.Asset, error) {
	return []connectorsdk.Asset{
		{
			ID:       "docker-ct-node-a-abc123",
			Type:     "docker-container",
			Name:     "nginx",
			Source:   "docker",
			Metadata: map[string]string{"cpu_percent": "7.5"},
		},
	}, nil
}
func (f *fakeConnector) TestConnection(context.Context) (connectorsdk.Health, error) {
	return connectorsdk.Health{Status: "ok", Message: "ok"}, nil
}
func (f *fakeConnector) Actions() []connectorsdk.ActionDescriptor {
	return []connectorsdk.ActionDescriptor{{ID: "container.start", Name: "Start Container", RequiresTarget: true}}
}
func (f *fakeConnector) ExecuteAction(_ context.Context, actionID string, _ connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	f.actionIDSeen = actionID
	return connectorsdk.ActionResult{Status: "succeeded", Message: "ok"}, nil
}

func TestWrapConnectorCanonicalizesDiscoverAndActions(t *testing.T) {
	t.Parallel()

	base := &fakeConnector{}
	wrapped := WrapConnector(base)

	assets, err := wrapped.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("len(assets) = %d, want 1", len(assets))
	}
	if assets[0].Class == "" {
		t.Fatalf("expected canonical class on discovered asset")
	}
	if assets[0].Kind != "docker-container" {
		t.Fatalf("assets[0].Kind = %q, want docker-container", assets[0].Kind)
	}
	if assets[0].Attributes == nil {
		t.Fatalf("expected derived canonical attributes")
	}

	actions := wrapped.Actions()
	if len(actions) != 1 {
		t.Fatalf("len(actions) = %d, want 1", len(actions))
	}
	if actions[0].CanonicalID != "workload.start" {
		t.Fatalf("actions[0].CanonicalID = %q, want workload.start", actions[0].CanonicalID)
	}
}

func TestWrapConnectorResolvesCanonicalActionID(t *testing.T) {
	t.Parallel()

	base := &fakeConnector{}
	wrapped := WrapConnector(base)

	_, err := wrapped.ExecuteAction(context.Background(), "workload.start", connectorsdk.ActionRequest{TargetID: "docker-ct-node-a-abc123"})
	if err != nil {
		t.Fatalf("ExecuteAction() error = %v", err)
	}
	if base.actionIDSeen != "container.start" {
		t.Fatalf("base.actionIDSeen = %q, want container.start", base.actionIDSeen)
	}
}
