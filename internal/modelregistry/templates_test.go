package modelregistry

import (
	"testing"
	"time"

	"github.com/labtether/labtether/internal/model"
)

func TestResolveTemplateBindingDockerContainer(t *testing.T) {
	binding := ResolveTemplateBinding(model.Resource{
		ID:     "docker-ct-1",
		Source: "docker",
		Class:  model.ResourceClassCompute,
		Kind:   "docker-container",
	}, []string{"logs.read"}, time.Now().UTC())

	if binding.TemplateID != "template.docker.container" {
		t.Fatalf("template_id=%q, want template.docker.container", binding.TemplateID)
	}
	if len(binding.Tabs) < 4 {
		t.Fatalf("tabs=%v, expected docker container tabs", binding.Tabs)
	}
	want := map[string]struct{}{"overview": {}, "logs": {}, "stats": {}, "inspect": {}}
	for tab := range want {
		if !containsString(binding.Tabs, tab) {
			t.Fatalf("expected tab %q in %v", tab, binding.Tabs)
		}
	}
}

func TestResolveTemplateBindingAddsCapabilityTabs(t *testing.T) {
	binding := ResolveTemplateBinding(model.Resource{
		ID:     "node-1",
		Source: "agent",
		Class:  model.ResourceClassCompute,
		Kind:   "host",
	}, []string{"service.list", "network.list", "terminal.open"}, time.Now().UTC())

	if !containsString(binding.Tabs, "services") {
		t.Fatalf("expected services tab in %v", binding.Tabs)
	}
	if !containsString(binding.Tabs, "interfaces") {
		t.Fatalf("expected interfaces tab in %v", binding.Tabs)
	}
	if !containsString(binding.Tabs, "terminal") {
		t.Fatalf("expected terminal tab in %v", binding.Tabs)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
