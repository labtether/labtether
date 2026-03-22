package main

import (
	"github.com/labtether/labtether/internal/assets"
	statusaggpkg "github.com/labtether/labtether/internal/hubapi/statusagg"
)

// status_aggregate_payloads.go — thin stub.
//
// All payload types and ETag logic have moved to internal/hubapi/statusagg/.
// Type aliases are provided here so existing callers in cmd/labtether/ compile
// without modification.

// Response payload type aliases.
type statusAggregateLiveSummary = statusaggpkg.LiveSummary
type statusAggregateLiveResponse = statusaggpkg.LiveResponse
type statusAggregateSummary = statusaggpkg.Summary
type statusAggregateResponse = statusaggpkg.Response
type statusCanonicalPayload = statusaggpkg.CanonicalPayload

// ETag and hash helpers — delegate to the canonical package.
func statusAggregateETag(response statusAggregateResponse) string {
	return statusaggpkg.ETag(response)
}

func statusETagMatches(headerValue, etag string) bool {
	return statusaggpkg.ETagMatches(headerValue, etag)
}

// Asset fingerprinting helpers.
func statusCanonicalAssetFingerprint(assetList []assets.Asset) string {
	return statusaggpkg.CanonicalAssetFingerprint(assetList)
}

func statusCanonicalAssetIDs(assetList []assets.Asset) []string {
	return statusaggpkg.CanonicalAssetIDs(assetList)
}
