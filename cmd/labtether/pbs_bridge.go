package main

import (
	"context"
	"net/http"

	"github.com/labtether/labtether/internal/assets"
	pbs "github.com/labtether/labtether/internal/connectors/pbs"
	pbspkg "github.com/labtether/labtether/internal/hubapi/pbs"
	"github.com/labtether/labtether/internal/hubcollector"
)

// buildPBSDeps constructs the pbspkg.Deps from the apiServer's fields.
func (s *apiServer) buildPBSDeps() *pbspkg.Deps {
	return &pbspkg.Deps{
		AssetStore:        s.assetStore,
		HubCollectorStore: s.hubCollectorStore,
		CredentialStore:   s.credentialStore,
		SecretsManager:    s.secretsManager,

		RequireAdminAuth: s.requireAdminAuth,

		WrapAuth:  s.withAuth,
		WrapAdmin: s.withAdminAuth,
	}
}

// Type aliases for PBS types used in cmd/labtether/.
type pbsRuntime = pbspkg.PBSRuntime
type cachedPBSRuntime = pbspkg.CachedPBSRuntime

// ensurePBSDeps returns pbsDeps, creating and caching on first call.
func (s *apiServer) ensurePBSDeps() *pbspkg.Deps {
	if s.pbsDeps != nil {
		d := s.pbsDeps
		d.AssetStore = s.assetStore
		d.HubCollectorStore = s.hubCollectorStore
		d.CredentialStore = s.credentialStore
		d.SecretsManager = s.secretsManager
		d.RequireAdminAuth = s.requireAdminAuth
		return d
	}
	d := s.buildPBSDeps()
	s.pbsDeps = d
	return d
}

// Forwarding methods.

func (s *apiServer) handlePBSAssets(w http.ResponseWriter, r *http.Request) {
	s.ensurePBSDeps().HandlePBSAssets(w, r)
}

func (s *apiServer) loadPBSRuntime(collectorID string) (*pbspkg.PBSRuntime, error) {
	return s.ensurePBSDeps().LoadPBSRuntime(collectorID)
}

func (s *apiServer) handlePBSTaskRoutes(w http.ResponseWriter, r *http.Request) {
	s.ensurePBSDeps().HandlePBSTaskRoutes(w, r)
}

func (s *apiServer) handlePBSTaskStatus(w http.ResponseWriter, r *http.Request) {
	s.ensurePBSDeps().HandlePBSTaskStatus(w, r)
}

func (s *apiServer) handlePBSTaskStop(w http.ResponseWriter, r *http.Request) {
	s.ensurePBSDeps().HandlePBSTaskStop(w, r)
}

func (s *apiServer) handlePBSTaskLog(w http.ResponseWriter, r *http.Request) {
	s.ensurePBSDeps().HandlePBSTaskLog(w, r)
}

func (s *apiServer) resolvePBSAsset(assetID string) (assets.Asset, error) {
	return s.ensurePBSDeps().ResolvePBSAsset(assetID)
}

func (s *apiServer) resolvePBSAssetRuntime(assetID string) (assets.Asset, *pbspkg.PBSRuntime, error) {
	return s.ensurePBSDeps().ResolvePBSAssetRuntime(assetID)
}

func (s *apiServer) loadPBSAssetDetails(ctx context.Context, asset assets.Asset, runtime *pbspkg.PBSRuntime) (pbspkg.PBSAssetDetailsResponse, error) {
	return s.ensurePBSDeps().LoadPBSAssetDetails(ctx, asset, runtime)
}

// Error variable aliases.
var (
	errPBSAssetNotFound = pbspkg.ErrPBSAssetNotFound
	errAssetNotPBS      = pbspkg.ErrAssetNotPBS
)

// Package-level function aliases.
func writePBSResolveError(w http.ResponseWriter, err error) { pbspkg.WritePBSResolveError(w, err) }

func pbsStoreFromAsset(asset assets.Asset) string {
	return pbspkg.PBSStoreFromAsset(asset)
}

func selectCollectorForPBSRuntime(collectors []hubcollector.Collector, collectorID string) *hubcollector.Collector {
	return pbspkg.SelectCollectorForPBSRuntime(collectors, collectorID)
}

func filterAndSortPBSTasks(tasks []pbs.Task, store string, limit int) []pbs.Task {
	return pbspkg.FilterAndSortPBSTasks(tasks, store, limit)
}

func dedupeNonEmptyWarnings(warnings []string) []string {
	return pbspkg.DedupeNonEmptyWarnings(warnings)
}

func parsePBSTaskPath(trimmed, action string) (string, string, bool) {
	return pbspkg.ParsePBSTaskPath(trimmed, action)
}

func loadPBSDatastoreSummary(ctx context.Context, client *pbs.Client, store string, usage pbs.DatastoreUsage) (pbspkg.PBSDatastoreSummary, []string, error) {
	return pbspkg.LoadPBSDatastoreSummary(ctx, client, store, usage)
}


// Additional type aliases for test-referenced types.
type pbsSnapshotsResponse = pbspkg.PBSSnapshotsResponse
type pbsBackupGroupEntry = pbspkg.PBSBackupGroupEntry
type pbsSnapshotEntry = pbspkg.PBSSnapshotEntry
type pbsVerificationResponse = pbspkg.PBSVerificationResponse
type pbsCapabilities = pbspkg.PBSCapabilities
type pbsDatastoreSummary = pbspkg.PBSDatastoreSummary
type pbsGroupsResponse = pbspkg.PBSGroupsResponse
func pbsNodeFromAsset(asset assets.Asset) string { return pbspkg.PBSNodeFromAsset(asset) }
