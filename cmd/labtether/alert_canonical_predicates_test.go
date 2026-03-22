package main

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/model"
	"github.com/labtether/labtether/internal/telemetry"
)

func TestResolveRuleTargetAssets_CanonicalKinds(t *testing.T) {
	sut := newTestAPIServer(t)

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-vm-101",
		Type:    "vm",
		Name:    "vm-101",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"resource_kind": "vm",
		},
	})
	if err != nil {
		t.Fatalf("upsert vm heartbeat: %v", err)
	}

	_, err = sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-ct-202",
		Type:    "container",
		Name:    "ct-202",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"resource_kind": "container",
		},
	})
	if err != nil {
		t.Fatalf("upsert container heartbeat: %v", err)
	}

	rule := alerts.Rule{
		ID:          "rule-mixed-kinds",
		TargetScope: alerts.TargetScopeAsset,
		Targets: []alerts.RuleTarget{
			{ID: "target-vm", Selector: map[string]any{"resource_kind": "vm"}},
			{ID: "target-container", Selector: map[string]any{"resource_kind": "container"}},
		},
	}

	resolvedTargets, err := sut.resolveRuleTargetAssets(rule, nil, false)
	if err != nil {
		t.Fatalf("resolve rule target assets: %v", err)
	}

	ids := make([]string, 0, len(resolvedTargets))
	for _, target := range resolvedTargets {
		ids = append(ids, target.ID)
	}
	expected := []string{"proxmox-ct-202", "proxmox-vm-101"}
	if !reflect.DeepEqual(ids, expected) {
		t.Fatalf("resolved IDs mismatch:\n  got:  %v\n  want: %v", ids, expected)
	}
}

func TestResolveRuleTargetAssets_GlobalRuleWithNonMatchingTargetsDoesNotFallbackToAllAssets(t *testing.T) {
	sut := newTestAPIServer(t)

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "node-a",
		Type:    "host",
		Name:    "node-a",
		Source:  "labtether-agent",
		Status:  "online",
		Metadata: map[string]string{
			"resource_kind": "host",
		},
	})
	if err != nil {
		t.Fatalf("upsert node-a heartbeat: %v", err)
	}

	_, err = sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "node-b",
		Type:    "host",
		Name:    "node-b",
		Source:  "labtether-agent",
		Status:  "online",
		Metadata: map[string]string{
			"resource_kind": "host",
		},
	})
	if err != nil {
		t.Fatalf("upsert node-b heartbeat: %v", err)
	}

	rule := alerts.Rule{
		ID:          "rule-global-non-matching-target",
		TargetScope: alerts.TargetScopeGlobal,
		Targets: []alerts.RuleTarget{
			{ID: "target-miss", Selector: map[string]any{"resource_kind": "vm"}},
		},
	}

	resolvedTargets, err := sut.resolveRuleTargetAssets(rule, nil, true)
	if err != nil {
		t.Fatalf("resolve rule target assets: %v", err)
	}
	if len(resolvedTargets) != 0 {
		t.Fatalf("expected 0 resolved targets for non-matching explicit target, got %d", len(resolvedTargets))
	}
}

func TestEvaluateMetricThreshold_WithCapabilitySelectorPredicate(t *testing.T) {
	sut := newTestAPIServer(t)
	now := time.Now().UTC()

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "cap-node-1",
		Type:    "host",
		Name:    "cap-node-1",
		Source:  "labtether-agent",
		Status:  "online",
	})
	if err != nil {
		t.Fatalf("upsert heartbeat: %v", err)
	}

	_, err = sut.canonicalStore.UpsertCapabilitySet(model.CapabilitySet{
		SubjectType: "resource",
		SubjectID:   "cap-node-1",
		Capabilities: []model.CapabilitySpec{
			{ID: "network.action", Scope: model.CapabilityScopeAction},
		},
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("upsert capability set: %v", err)
	}

	err = sut.telemetryStore.AppendSamples(context.Background(), []telemetry.MetricSample{
		{
			AssetID:     "cap-node-1",
			Metric:      telemetry.MetricCPUUsedPercent,
			Unit:        "percent",
			Value:       96,
			CollectedAt: now.Add(-10 * time.Second),
		},
	})
	if err != nil {
		t.Fatalf("append telemetry sample: %v", err)
	}

	rule := alerts.Rule{
		ID:            "rule-cap-selector",
		Kind:          alerts.RuleKindMetricThreshold,
		TargetScope:   alerts.TargetScopeAsset,
		WindowSeconds: 300,
		Condition: map[string]any{
			"metric":    telemetry.MetricCPUUsedPercent,
			"operator":  ">",
			"threshold": 90,
			"aggregate": "max",
		},
		Targets: []alerts.RuleTarget{
			{ID: "target-selector", Selector: map[string]any{"capability": "network.action"}},
		},
	}

	triggered, err := sut.evaluateMetricThreshold(context.Background(), rule)
	if err != nil {
		t.Fatalf("evaluate metric threshold: %v", err)
	}
	if !triggered {
		t.Fatalf("expected capability selector metric rule to trigger")
	}
}

func TestValidateAlertRuleTargets_RejectsDeprecatedSelectorKeys(t *testing.T) {
	sut := newTestAPIServer(t)

	err := sut.validateAlertRuleTargets([]alerts.RuleTargetInput{
		{
			AssetID: "",
			Selector: map[string]any{
				"kind": "vm",
			},
		},
	})
	if err == nil {
		t.Fatalf("expected deprecated selector key validation error")
	}
	if !strings.Contains(err.Error(), "deprecated") {
		t.Fatalf("expected deprecated selector key error, got %v", err)
	}
}
