package main

import (
	"context"
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/assets"
	tnconnector "github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/hubapi/shared"
	truenaspkg "github.com/labtether/labtether/internal/hubapi/truenas"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/terminal"
)

// buildTruenasDeps constructs the truenaspkg.Deps from the apiServer's fields.
func (s *apiServer) buildTruenasDeps() *truenaspkg.Deps {
	return &truenaspkg.Deps{
		AssetStore:        s.assetStore,
		HubCollectorStore: s.hubCollectorStore,
		CredentialStore:   s.credentialStore,
		SecretsManager:    s.secretsManager,
		LogStore:          s.logStore,

		TruenasSubs: make(map[string]truenaspkg.TruenasSubscriptionHandle, 8),

		RequireAdminAuth: s.requireAdminAuth,

		WrapAuth:  s.withAuth,
		WrapAdmin: s.withAdminAuth,

		CheckSameOrigin: checkSameOrigin,

		AppendConnectorLogEvent:       s.appendConnectorLogEvent,
		AppendConnectorLogEventWithID: s.appendConnectorLogEventWithID,

		StartBrowserWebSocketKeepalive:    startBrowserWebSocketKeepalive,
		TouchBrowserWebSocketReadDeadline: touchBrowserWebSocketReadDeadline,
	}
}

// Type aliases for truenas types used in cmd/labtether/.
type truenasRuntime = truenaspkg.TruenasRuntime
type cachedTrueNASRuntime = truenaspkg.CachedTrueNASRuntime
type truenasSubscriptionHandle = truenaspkg.TruenasSubscriptionHandle
type truenasShellTarget = truenaspkg.TruenasShellTarget
type trueNASAssetSMARTResponse = truenaspkg.TrueNASAssetSMARTResponse
type trueNASFilesystemResponse = truenaspkg.TrueNASFilesystemResponse
type trueNASAssetEventsResponse = truenaspkg.TrueNASAssetEventsResponse
type truenasCapabilities = truenaspkg.TruenasCapabilities

type trueNASDiskHealth = truenaspkg.TrueNASDiskHealth
type trueNASServiceEntry = truenaspkg.TrueNASServiceEntry

// Exported error variables.
var (
	errTrueNASAssetNotFound = truenaspkg.ErrTrueNASAssetNotFound
	errAssetNotTrueNAS      = truenaspkg.ErrAssetNotTrueNAS
)

// Package-level function aliases for tests.
func writeTrueNASResolveError(w http.ResponseWriter, err error) {
	truenaspkg.WriteTrueNASResolveError(w, err)
}

func latestSmartResultsByDisk(results []map[string]any) map[string]map[string]any {
	return truenaspkg.LatestSmartResultsByDisk(results)
}

func deriveTrueNASDiskHealthStatus(view truenaspkg.TrueNASDiskHealth) string {
	return truenaspkg.DeriveTrueNASDiskHealthStatus(view)
}

func trueNASDiskHealthSeverity(status string) int {
	return truenaspkg.TrueNASDiskHealthSeverity(status)
}

func normalizeTrueNASFilesystemPath(raw string) string {
	return truenaspkg.NormalizeTrueNASFilesystemPath(raw)
}

func parentTrueNASFilesystemPath(currentPath string) string {
	return truenaspkg.ParentTrueNASFilesystemPath(currentPath)
}

func mapTrueNASServiceEntry(svc map[string]any) truenaspkg.TrueNASServiceEntry {
	return truenaspkg.MapTrueNASServiceEntry(svc)
}

func staleTrueNASReadWarning(message, fetchedAt string) string {
	return truenaspkg.StaleTrueNASReadWarning(message, fetchedAt)
}

func appendTrueNASWarning(existing []string, warning string) []string {
	return truenaspkg.AppendTrueNASWarning(existing, warning)
}

// ensureTruenasDeps returns truenasDeps. When pre-initialized (production),
// returns the cached instance. Otherwise, rebuilds on every call so that test
// mutations to apiServer store fields are visible, while reusing the cached
// Deps's mutable state (caches, subscription handles) for state continuity.
func (s *apiServer) ensureTruenasDeps() *truenaspkg.Deps {
	if s.truenasDeps != nil {
		// Production path: update store pointers from apiServer so test
		// mutations are reflected without losing cached state.
		d := s.truenasDeps
		d.AssetStore = s.assetStore
		d.HubCollectorStore = s.hubCollectorStore
		d.CredentialStore = s.credentialStore
		d.SecretsManager = s.secretsManager
		d.LogStore = s.logStore
		d.RequireAdminAuth = s.requireAdminAuth
		return d
	}
	d := s.buildTruenasDeps()
	s.truenasDeps = d
	return d
}

// Forwarding methods from apiServer to truenaspkg.Deps so that existing
// cmd/labtether/ callers and tests keep compiling without changes.

func (s *apiServer) handleTrueNASAssets(w http.ResponseWriter, r *http.Request) {
	s.ensureTruenasDeps().HandleTrueNASAssets(w, r)
}

func (s *apiServer) handleTrueNASCapabilities(ctx context.Context, w http.ResponseWriter, asset assets.Asset, runtime *truenaspkg.TruenasRuntime) {
	s.ensureTruenasDeps().HandleTrueNASCapabilities(ctx, w, asset, runtime)
}

func (s *apiServer) handleTrueNASDisks(ctx context.Context, w http.ResponseWriter, r *http.Request, asset assets.Asset, runtime *truenaspkg.TruenasRuntime, subParts []string) {
	s.ensureTruenasDeps().HandleTrueNASDisks(ctx, w, r, asset, runtime, subParts)
}

func (s *apiServer) resolveTrueNASSessionTarget(assetID string) (truenaspkg.TruenasShellTarget, bool, error) {
	return s.ensureTruenasDeps().ResolveTrueNASSessionTarget(assetID)
}

func (s *apiServer) tryTrueNASTerminalStream(w http.ResponseWriter, r *http.Request, session terminal.Session, target truenaspkg.TruenasShellTarget) error {
	return s.ensureTruenasDeps().TryTrueNASTerminalStream(w, r, session, target)
}

func (s *apiServer) loadTrueNASRuntime(collectorID string) (*truenaspkg.TruenasRuntime, error) {
	return s.ensureTruenasDeps().LoadTrueNASRuntime(collectorID)
}

func (s *apiServer) ensureTrueNASSubscriptionWorker(ctx context.Context, collector hubcollector.Collector, runtime *truenaspkg.TruenasRuntime) {
	s.ensureTruenasDeps().EnsureTrueNASSubscriptionWorker(ctx, collector, runtime)
}

func (s *apiServer) resolveTrueNASAsset(assetID string) (assets.Asset, error) {
	return s.ensureTruenasDeps().ResolveTrueNASAsset(assetID)
}

func (s *apiServer) resolveTrueNASAssetRuntime(assetID string) (assets.Asset, *truenaspkg.TruenasRuntime, error) {
	return s.ensureTruenasDeps().ResolveTrueNASAssetRuntime(assetID)
}

func (s *apiServer) invalidateTrueNASCaches(assetID, collectorID string) {
	s.ensureTruenasDeps().InvalidateTrueNASCaches(assetID, collectorID)
}

func (s *apiServer) getCachedTrueNASSMART(assetID, collectorID string) (truenaspkg.TrueNASAssetSMARTResponse, bool) {
	return s.ensureTruenasDeps().GetCachedTrueNASSMART(assetID, collectorID)
}

func (s *apiServer) setCachedTrueNASSMART(assetID, collectorID string, response truenaspkg.TrueNASAssetSMARTResponse) {
	s.ensureTruenasDeps().SetCachedTrueNASSMART(assetID, collectorID, response)
}

func (s *apiServer) getCachedTrueNASFilesystem(assetID, collectorID, requestPath string) (truenaspkg.TrueNASFilesystemResponse, bool) {
	return s.ensureTruenasDeps().GetCachedTrueNASFilesystem(assetID, collectorID, requestPath)
}

func (s *apiServer) setCachedTrueNASFilesystem(assetID, collectorID, requestPath string, response truenaspkg.TrueNASFilesystemResponse) {
	s.ensureTruenasDeps().SetCachedTrueNASFilesystem(assetID, collectorID, requestPath, response)
}

func (s *apiServer) ingestTrueNASSubscriptionEvent(collector hubcollector.Collector, collectorID string, event tnconnector.SubscriptionEvent) {
	s.ensureTruenasDeps().IngestTrueNASSubscriptionEvent(collector, collectorID, event)
}

func (s *apiServer) runTrueNASSubscriptionWorker(ctx context.Context, collector hubcollector.Collector, runtime *truenaspkg.TruenasRuntime) {
	s.ensureTruenasDeps().RunTrueNASSubscriptionWorker(ctx, collector, runtime)
}

func (s *apiServer) unregisterTrueNASSubscriptionWorker(collectorID, configKey string) {
	s.ensureTruenasDeps().UnregisterTrueNASSubscriptionWorker(collectorID, configKey)
}

func (s *apiServer) isTrueNASCollectorActive(collectorID string) bool {
	return s.ensureTruenasDeps().IsTrueNASCollectorActive(collectorID)
}

func (s *apiServer) handleTrueNASAssetEvents(w http.ResponseWriter, r *http.Request, assetID string) {
	s.ensureTruenasDeps().HandleTrueNASAssetEvents(w, r, assetID)
}

// Package-level function aliases.

func selectCollectorForTrueNASRuntime(collectors []hubcollector.Collector, collectorID string) *hubcollector.Collector {
	return truenaspkg.SelectCollectorForTrueNASRuntime(collectors, collectorID)
}

func parseTrueNASEventsWindow(raw string) time.Duration {
	return truenaspkg.ParseTrueNASEventsWindow(raw)
}

func truenasShellEndpoint(baseURL string) string {
	return truenaspkg.TruenasShellEndpoint(baseURL)
}

func normalizeTrueNASSubscriptionFields(fields map[string]any) map[string]string {
	return truenaspkg.NormalizeTrueNASSubscriptionFields(fields)
}

func copySubscriptionField(target map[string]string, source map[string]string, key string) {
	truenaspkg.CopySubscriptionField(target, source, key)
}

func trueNASSubscriptionMessage(event tnconnector.SubscriptionEvent, fields map[string]string) string {
	return truenaspkg.TrueNASSubscriptionMessage(event, fields)
}

func normalizeTrueNASListDirResult(result any) ([]map[string]any, bool) {
	return truenaspkg.NormalizeTrueNASListDirResult(result)
}

func mapTrueNASFilesystemEntry(entry map[string]any, basePath string) truenaspkg.TrueNASFilesystemEntry {
	return truenaspkg.MapTrueNASFilesystemEntry(entry, basePath)
}

func callTrueNASQueryCompat(ctx context.Context, client *tnconnector.Client, method string, dest any) error {
	return truenaspkg.CallTrueNASQueryCompat(ctx, client, method, dest)
}

func callTrueNASListDir(ctx context.Context, client *tnconnector.Client, directoryPath string) ([]map[string]any, error) {
	return truenaspkg.CallTrueNASListDir(ctx, client, directoryPath)
}

func callTrueNASListDirWithRetries(ctx context.Context, client *tnconnector.Client, directoryPath string) ([]map[string]any, error) {
	return truenaspkg.CallTrueNASListDirWithRetries(ctx, client, directoryPath)
}

func callTrueNASMethodWithRetries(ctx context.Context, client *tnconnector.Client, method string, params []any, dest any) error {
	return truenaspkg.CallTrueNASMethodWithRetries(ctx, client, method, params, dest)
}

func callTrueNASQueryWithRetries(ctx context.Context, client *tnconnector.Client, method string, dest any) error {
	return truenaspkg.CallTrueNASQueryWithRetries(ctx, client, method, dest)
}

func waitForTrueNASMethodRetry(ctx context.Context, attempt int) bool {
	return truenaspkg.WaitForTrueNASMethodRetry(ctx, attempt)
}

func trueNASSmartAssetCacheKey(assetID string) string {
	return truenaspkg.TrueNASSmartAssetCacheKey(assetID)
}

func trueNASSmartCollectorCacheKey(collectorID string) string {
	return truenaspkg.TrueNASSmartCollectorCacheKey(collectorID)
}

func trueNASFilesystemCacheKey(scope, id, requestPath string) string {
	return truenaspkg.TrueNASFilesystemCacheKey(scope, id, requestPath)
}

// Utility functions that were in truenas source but now in shared.
// Some (parsePositiveInt, parseAnyInt64, formatStorageInsightsWindow) are
// already declared in proxmox_bridge.go; only declare the new ones here.
func anyToFloat64(value any) float64                { return shared.AnyToFloat64(value) }
func parseAnyBoolLoose(value any) (bool, bool)      { return shared.ParseAnyBoolLoose(value) }
func parseAnyTimestamp(value any) (time.Time, bool) { return shared.ParseAnyTimestamp(value) }
