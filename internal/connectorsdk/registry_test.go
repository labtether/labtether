package connectorsdk

import (
	"context"
	"testing"
)

type fakeConnector struct{ id string }

func (f fakeConnector) ID() string                 { return f.id }
func (f fakeConnector) DisplayName() string        { return f.id }
func (f fakeConnector) Capabilities() Capabilities { return Capabilities{DiscoverAssets: true} }
func (f fakeConnector) Discover(context.Context) ([]Asset, error) {
	return []Asset{{ID: "a1", Source: f.id}}, nil
}
func (f fakeConnector) TestConnection(context.Context) (Health, error) {
	return Health{Status: "ok"}, nil
}
func (f fakeConnector) Actions() []ActionDescriptor { return nil }
func (f fakeConnector) ExecuteAction(context.Context, string, ActionRequest) (ActionResult, error) {
	return ActionResult{Status: "succeeded", Message: "ok"}, nil
}

func TestRegistryRegisterAndList(t *testing.T) {
	reg := NewRegistry()
	reg.Register(fakeConnector{id: "z"})
	reg.Register(fakeConnector{id: "a"})

	list := reg.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 connectors, got %d", len(list))
	}
	if list[0].ID != "a" {
		t.Fatalf("expected sorted list, got %s first", list[0].ID)
	}
}
