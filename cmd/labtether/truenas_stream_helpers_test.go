package main

import (
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubcollector"
)

func TestResolveTrueNASSessionTargetUsesCollectorMetadata(t *testing.T) {
	sut := newTestAPIServer(t)
	createTrueNASCredentialProfile(t, sut, "cred-truenas-1", "api-key-1", "https://truenas-a.local")
	createTrueNASCredentialProfile(t, sut, "cred-truenas-2", "api-key-2", "https://truenas-b.local")
	configureTrueNASCollectors(t, sut,
		hubcollector.Collector{
			ID:            "collector-truenas-1",
			AssetID:       "truenas-cluster-1",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config:        map[string]any{"base_url": "https://truenas-a.local", "credential_id": "cred-truenas-1", "skip_verify": true},
		},
		hubcollector.Collector{
			ID:            "collector-truenas-2",
			AssetID:       "truenas-cluster-2",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config:        map[string]any{"base_url": "https://truenas-b.local", "credential_id": "cred-truenas-2", "skip_verify": false},
		},
	)

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "truenas-vm-101",
		Type:    "vm",
		Name:    "tn-vm-101",
		Source:  "truenas",
		Status:  "online",
		Metadata: map[string]string{
			"collector_id": "collector-truenas-2",
			"vm_id":        "101",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed vm asset: %v", err)
	}

	target, ok, err := sut.resolveTrueNASSessionTarget("truenas-vm-101")
	if err != nil {
		t.Fatalf("resolveTrueNASSessionTarget returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected truenas target resolution")
	}
	if got, want := strings.TrimSpace(target.BaseURL), "https://truenas-b.local"; got != want {
		t.Fatalf("target base_url = %q, want %q", got, want)
	}
	if target.SkipVerify {
		t.Fatalf("expected skip_verify false from collector-truenas-2")
	}
	if target.Options == nil || target.Options["vm_id"] != "101" {
		t.Fatalf("expected vm_id option 101, got %#v", target.Options)
	}
}

func TestLoadTrueNASRuntimeCacheHit(t *testing.T) {
	sut := newTestAPIServer(t)
	createTrueNASCredentialProfile(t, sut, "cred-truenas-cache", "api-key-cache", "https://truenas-cache.local")
	configureTrueNASCollectors(t, sut, hubcollector.Collector{
		ID:            "collector-truenas-cache",
		AssetID:       "truenas-cluster-cache",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      "https://truenas-cache.local",
			"credential_id": "cred-truenas-cache",
			"skip_verify":   true,
		},
	})

	first, err := sut.loadTrueNASRuntime("collector-truenas-cache")
	if err != nil {
		t.Fatalf("first loadTrueNASRuntime failed: %v", err)
	}
	second, err := sut.loadTrueNASRuntime("collector-truenas-cache")
	if err != nil {
		t.Fatalf("second loadTrueNASRuntime failed: %v", err)
	}
	if first != second {
		t.Fatalf("expected runtime cache hit to reuse pointer")
	}
	if first.Client == nil {
		t.Fatalf("expected runtime client to be initialized")
	}
}
