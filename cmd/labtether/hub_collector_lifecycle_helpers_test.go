package main

import (
	"testing"

	"github.com/labtether/labtether/internal/hubcollector"
)

func TestKeepConnectorClusterAssetAliveUpsertsClusterHeartbeat(t *testing.T) {
	sut := newTestAPIServer(t)

	collector := hubcollector.Collector{
		ID:            "collector-cluster-refresh",
		AssetID:       "truenas-cluster-refresh",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
	}

	snapshotAsset, ok := sut.keepConnectorClusterAssetAlive(collector, "truenas", 3, "hub collector truenas")
	if !ok {
		t.Fatalf("expected keepConnectorClusterAssetAlive to succeed")
	}
	if snapshotAsset.ID != collector.AssetID {
		t.Fatalf("snapshot asset ID = %q, want %q", snapshotAsset.ID, collector.AssetID)
	}
	if snapshotAsset.Type != "connector-cluster" {
		t.Fatalf("snapshot asset Type = %q, want connector-cluster", snapshotAsset.Type)
	}
	if snapshotAsset.Metadata["connector_type"] != "truenas" {
		t.Fatalf("snapshot connector_type = %q, want truenas", snapshotAsset.Metadata["connector_type"])
	}
	if snapshotAsset.Metadata["discovered"] != "3" {
		t.Fatalf("snapshot discovered metadata = %q, want 3", snapshotAsset.Metadata["discovered"])
	}
	if snapshotAsset.Metadata["collector_id"] != collector.ID {
		t.Fatalf("snapshot collector_id = %q, want %q", snapshotAsset.Metadata["collector_id"], collector.ID)
	}

	clusterAsset, exists, err := sut.assetStore.GetAsset(collector.AssetID)
	if err != nil {
		t.Fatalf("GetAsset() error = %v", err)
	}
	if !exists {
		t.Fatalf("expected refreshed cluster asset to be stored")
	}
	if clusterAsset.Type != "connector-cluster" {
		t.Fatalf("stored cluster asset type = %q, want connector-cluster", clusterAsset.Type)
	}
	if clusterAsset.Metadata["discovered"] != "3" {
		t.Fatalf("stored discovered metadata = %q, want 3", clusterAsset.Metadata["discovered"])
	}
	if clusterAsset.Metadata["collector_id"] != collector.ID {
		t.Fatalf("stored collector_id metadata = %q, want %q", clusterAsset.Metadata["collector_id"], collector.ID)
	}
}

func TestRefreshCollectorParentAssetSupportsCustomRootType(t *testing.T) {
	sut := newTestAPIServer(t)

	collector := hubcollector.Collector{
		ID:            "collector-pbs-root",
		AssetID:       "pbs-root-asset",
		CollectorType: hubcollector.CollectorTypePBS,
		Enabled:       true,
	}

	snapshotAsset, ok := sut.refreshCollectorParentAsset(collectorParentAssetRefreshOptions{
		Collector: collector,
		Source:    "pbs",
		AssetType: "storage-controller",
		Name:      "pbs-root",
		Status:    "online",
		Metadata: map[string]string{
			"connector_type": "pbs",
			"discovered":     "5",
		},
		LogPrefix:      "hub collector pbs",
		FailureSubject: "collector root asset",
	})
	if !ok {
		t.Fatalf("expected refreshCollectorParentAsset to succeed")
	}
	if snapshotAsset.Type != "storage-controller" {
		t.Fatalf("snapshot Type = %q, want storage-controller", snapshotAsset.Type)
	}
	if snapshotAsset.Name != "pbs-root" {
		t.Fatalf("snapshot Name = %q, want pbs-root", snapshotAsset.Name)
	}
	if snapshotAsset.Metadata["connector_type"] != "pbs" {
		t.Fatalf("snapshot connector_type = %q, want pbs", snapshotAsset.Metadata["connector_type"])
	}
	if snapshotAsset.Metadata["collector_id"] != collector.ID {
		t.Fatalf("snapshot collector_id = %q, want %q", snapshotAsset.Metadata["collector_id"], collector.ID)
	}
}
