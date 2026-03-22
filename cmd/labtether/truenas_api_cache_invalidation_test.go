package main

import (
	"testing"
)

// TestInvalidateTrueNASCachesClearsSMARTEntries verifies that
// invalidateTrueNASCaches removes both the asset-keyed and collector-keyed
// SMART cache entries without touching entries for unrelated assets.
func TestInvalidateTrueNASCachesClearsSMARTEntries(t *testing.T) {
	sut := &apiServer{}

	// Seed SMART cache entries for two different assets.
	sut.setCachedTrueNASSMART("asset-a", "collector-a", trueNASAssetSMARTResponse{AssetID: "asset-a"})
	sut.setCachedTrueNASSMART("asset-b", "collector-b", trueNASAssetSMARTResponse{AssetID: "asset-b"})

	sut.invalidateTrueNASCaches("asset-a", "collector-a")

	if _, ok := sut.getCachedTrueNASSMART("asset-a", ""); ok {
		t.Fatal("expected asset-a SMART cache entry to be cleared after invalidation")
	}
	if _, ok := sut.getCachedTrueNASSMART("", "collector-a"); ok {
		t.Fatal("expected collector-a SMART cache entry to be cleared after invalidation")
	}
	if _, ok := sut.getCachedTrueNASSMART("asset-b", "collector-b"); !ok {
		t.Fatal("expected unrelated asset-b SMART cache entry to remain after invalidating asset-a")
	}
}

// TestInvalidateTrueNASCachesClearsFilesystemEntries verifies that all
// filesystem cache entries keyed to the invalidated asset (across different
// paths) are removed without affecting entries for a different asset.
func TestInvalidateTrueNASCachesClearsFilesystemEntries(t *testing.T) {
	sut := &apiServer{}

	// Seed several filesystem cache entries under asset-a (different paths).
	sut.setCachedTrueNASFilesystem("asset-a", "collector-a", "/mnt", trueNASFilesystemResponse{AssetID: "asset-a", Path: "/mnt"})
	sut.setCachedTrueNASFilesystem("asset-a", "collector-a", "/mnt/data", trueNASFilesystemResponse{AssetID: "asset-a", Path: "/mnt/data"})
	// Seed an entry for an unrelated asset.
	sut.setCachedTrueNASFilesystem("asset-b", "collector-b", "/mnt", trueNASFilesystemResponse{AssetID: "asset-b", Path: "/mnt"})

	sut.invalidateTrueNASCaches("asset-a", "collector-a")

	if _, ok := sut.getCachedTrueNASFilesystem("asset-a", "", "/mnt"); ok {
		t.Fatal("expected /mnt filesystem cache for asset-a to be cleared after invalidation")
	}
	if _, ok := sut.getCachedTrueNASFilesystem("asset-a", "", "/mnt/data"); ok {
		t.Fatal("expected /mnt/data filesystem cache for asset-a to be cleared after invalidation")
	}
	if _, ok := sut.getCachedTrueNASFilesystem("asset-b", "collector-b", "/mnt"); !ok {
		t.Fatal("expected unrelated asset-b filesystem cache entry to remain after invalidating asset-a")
	}
}

// TestInvalidateTrueNASCachesHandlesNilCaches verifies that calling
// invalidateTrueNASCaches when no cache has been initialised does not panic.
func TestInvalidateTrueNASCachesHandlesNilCaches(t *testing.T) {
	sut := &apiServer{}
	// Neither truenasSmartCache nor truenasFSCache has been initialised.
	sut.invalidateTrueNASCaches("asset-x", "collector-x") // must not panic
}

// TestInvalidateTrueNASCachesHandlesEmptyIDs verifies that calling
// invalidateTrueNASCaches with empty IDs does not panic and does not clear
// entries for other assets.
func TestInvalidateTrueNASCachesHandlesEmptyIDs(t *testing.T) {
	sut := &apiServer{}
	sut.setCachedTrueNASSMART("asset-a", "collector-a", trueNASAssetSMARTResponse{AssetID: "asset-a"})
	sut.invalidateTrueNASCaches("", "") // must not panic and must leave asset-a intact
	if _, ok := sut.getCachedTrueNASSMART("asset-a", "collector-a"); !ok {
		t.Fatal("expected asset-a SMART cache entry to remain when invalidateTrueNASCaches is called with empty IDs")
	}
}
