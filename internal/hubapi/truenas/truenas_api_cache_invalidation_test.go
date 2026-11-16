package truenas_test

import (
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
	truenaspkg "github.com/labtether/labtether/internal/hubapi/truenas"
	"testing"
)

// TestInvalidateTrueNASCachesClearsSMARTEntries verifies that
// invalidateTrueNASCaches removes both the asset-keyed and collector-keyed
// SMART cache entries without touching entries for unrelated assets.
func TestInvalidateTrueNASCachesClearsSMARTEntries(t *testing.T) {
	sut := hubdeps.NewEmpty(t)

	// Seed SMART cache entries for two different assets.
	sut.TrueNASDeps().SetCachedTrueNASSMART("asset-a", "collector-a", truenaspkg.TrueNASAssetSMARTResponse{AssetID: "asset-a"})
	sut.TrueNASDeps().SetCachedTrueNASSMART("asset-b", "collector-b", truenaspkg.TrueNASAssetSMARTResponse{AssetID: "asset-b"})

	sut.TrueNASDeps().InvalidateTrueNASCaches("asset-a", "collector-a")

	if _, ok := sut.TrueNASDeps().GetCachedTrueNASSMART("asset-a", ""); ok {
		t.Fatal("expected asset-a SMART cache entry to be cleared after invalidation")
	}
	if _, ok := sut.TrueNASDeps().GetCachedTrueNASSMART("", "collector-a"); ok {
		t.Fatal("expected collector-a SMART cache entry to be cleared after invalidation")
	}
	if _, ok := sut.TrueNASDeps().GetCachedTrueNASSMART("asset-b", "collector-b"); !ok {
		t.Fatal("expected unrelated asset-b SMART cache entry to remain after invalidating asset-a")
	}
}

// TestInvalidateTrueNASCachesClearsFilesystemEntries verifies that all
// filesystem cache entries keyed to the invalidated asset (across different
// paths) are removed without affecting entries for a different asset.
func TestInvalidateTrueNASCachesClearsFilesystemEntries(t *testing.T) {
	sut := hubdeps.NewEmpty(t)

	// Seed several filesystem cache entries under asset-a (different paths).
	sut.TrueNASDeps().SetCachedTrueNASFilesystem("asset-a", "collector-a", "/mnt", truenaspkg.TrueNASFilesystemResponse{AssetID: "asset-a", Path: "/mnt"})
	sut.TrueNASDeps().SetCachedTrueNASFilesystem("asset-a", "collector-a", "/mnt/data", truenaspkg.TrueNASFilesystemResponse{AssetID: "asset-a", Path: "/mnt/data"})
	// Seed an entry for an unrelated asset.
	sut.TrueNASDeps().SetCachedTrueNASFilesystem("asset-b", "collector-b", "/mnt", truenaspkg.TrueNASFilesystemResponse{AssetID: "asset-b", Path: "/mnt"})

	sut.TrueNASDeps().InvalidateTrueNASCaches("asset-a", "collector-a")

	if _, ok := sut.TrueNASDeps().GetCachedTrueNASFilesystem("asset-a", "", "/mnt"); ok {
		t.Fatal("expected /mnt filesystem cache for asset-a to be cleared after invalidation")
	}
	if _, ok := sut.TrueNASDeps().GetCachedTrueNASFilesystem("asset-a", "", "/mnt/data"); ok {
		t.Fatal("expected /mnt/data filesystem cache for asset-a to be cleared after invalidation")
	}
	if _, ok := sut.TrueNASDeps().GetCachedTrueNASFilesystem("asset-b", "collector-b", "/mnt"); !ok {
		t.Fatal("expected unrelated asset-b filesystem cache entry to remain after invalidating asset-a")
	}
}

// TestInvalidateTrueNASCachesHandlesNilCaches verifies that calling
// invalidateTrueNASCaches when no cache has been initialised does not panic.
func TestInvalidateTrueNASCachesHandlesNilCaches(t *testing.T) {
	sut := hubdeps.NewEmpty(t)
	// Neither truenasSmartCache nor truenasFSCache has been initialised.
	sut.TrueNASDeps().InvalidateTrueNASCaches("asset-x", "collector-x") // must not panic
}

// TestInvalidateTrueNASCachesHandlesEmptyIDs verifies that calling
// invalidateTrueNASCaches with empty IDs does not panic and does not clear
// entries for other assets.
func TestInvalidateTrueNASCachesHandlesEmptyIDs(t *testing.T) {
	sut := hubdeps.NewEmpty(t)
	sut.TrueNASDeps().SetCachedTrueNASSMART("asset-a", "collector-a", truenaspkg.TrueNASAssetSMARTResponse{AssetID: "asset-a"})
	sut.TrueNASDeps().InvalidateTrueNASCaches("", "") // must not panic and must leave asset-a intact
	if _, ok := sut.TrueNASDeps().GetCachedTrueNASSMART("asset-a", "collector-a"); !ok {
		t.Fatal("expected asset-a SMART cache entry to remain when invalidateTrueNASCaches is called with empty IDs")
	}
}
