package persistence

import (
	"testing"
	"time"

	"github.com/labtether/labtether/internal/model"
)

func TestMemoryCanonicalStoreReplaceCapabilitySets(t *testing.T) {
	store := NewMemoryCanonicalModelStore()
	providerID := "prov-connector-docker-main"

	if _, err := store.UpsertProviderInstance(model.ProviderInstance{
		ID:          providerID,
		Kind:        model.ProviderKindConnector,
		Provider:    "docker",
		Status:      model.ProviderStatusHealthy,
		Scope:       model.ProviderScopeGlobal,
		DisplayName: "Docker",
		LastSeenAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert provider: %v", err)
	}

	err := store.ReplaceCapabilitySets(providerID, []model.CapabilitySet{
		{
			SubjectType: "provider",
			SubjectID:   providerID,
			Capabilities: []model.CapabilitySpec{
				{ID: "inventory.discover", Scope: model.CapabilityScopeRead},
			},
		},
		{
			SubjectType: "resource",
			SubjectID:   "docker-host-lab",
			Capabilities: []model.CapabilitySpec{
				{ID: "service.list", Scope: model.CapabilityScopeRead},
			},
		},
	})
	if err != nil {
		t.Fatalf("replace capability sets: %v", err)
	}

	sets, err := store.ListCapabilitySets(20)
	if err != nil {
		t.Fatalf("list capability sets: %v", err)
	}
	if len(sets) != 2 {
		t.Fatalf("len(sets)=%d, want 2", len(sets))
	}

	if err := store.ReplaceCapabilitySets(providerID, nil); err != nil {
		t.Fatalf("replace capability sets clear: %v", err)
	}
	sets, err = store.ListCapabilitySets(20)
	if err != nil {
		t.Fatalf("list capability sets after clear: %v", err)
	}
	if len(sets) != 0 {
		t.Fatalf("len(sets) after clear=%d, want 0", len(sets))
	}
}

func TestMemoryCanonicalStoreTemplateBindingsFilter(t *testing.T) {
	store := NewMemoryCanonicalModelStore()
	now := time.Now().UTC()

	if _, err := store.UpsertTemplateBinding(model.TemplateBinding{
		ResourceID: "asset-1",
		TemplateID: "template.compute.default",
		Tabs:       []string{"overview", "telemetry"},
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("upsert template binding 1: %v", err)
	}
	if _, err := store.UpsertTemplateBinding(model.TemplateBinding{
		ResourceID: "asset-2",
		TemplateID: "template.storage.default",
		Tabs:       []string{"overview", "logs"},
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("upsert template binding 2: %v", err)
	}

	bindings, err := store.ListTemplateBindings([]string{"asset-2"})
	if err != nil {
		t.Fatalf("list template bindings: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("len(bindings)=%d, want 1", len(bindings))
	}
	if bindings[0].ResourceID != "asset-2" {
		t.Fatalf("resource_id=%q, want asset-2", bindings[0].ResourceID)
	}
}
