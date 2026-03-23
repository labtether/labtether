package statusagg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

// etagCache is a per-handler in-memory cache mapping (groupFilter|scopeKey)
// -> (fingerprint, etag) so repeated requests with the same fingerprint can
// short-circuit the full response build.
type etagCache struct {
	mu      sync.RWMutex
	entries map[string]ETagCacheEntry
}

func newETagCache() *etagCache {
	return &etagCache{
		entries: make(map[string]ETagCacheEntry, 4),
	}
}

func (c *etagCache) match(cacheKey, fingerprint string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[cacheKey]
	if !ok || entry.Fingerprint != fingerprint || strings.TrimSpace(entry.ETag) == "" {
		return "", false
	}
	return entry.ETag, true
}

func (c *etagCache) store(cacheKey, fingerprint, etag string) {
	etag = strings.TrimSpace(etag)
	if etag == "" || strings.TrimSpace(fingerprint) == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[cacheKey] = ETagCacheEntry{
		Fingerprint: fingerprint,
		ETag:        etag,
	}
}

func (c *etagCache) hasKey(cacheKey string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.entries[cacheKey]
	return ok
}

// Fingerprint computes a fast fingerprint from the given parts using SHA-256.
// It is used as a cheap pre-check before building the full response, avoiding
// unnecessary DB queries when nothing has changed.
func Fingerprint(parts []string) string {
	h := sha256.New()
	for idx, part := range parts {
		if idx > 0 {
			_, _ = h.Write([]byte{0})
		}
		_, _ = io.WriteString(h, part) // #nosec G705 -- Hashing internal identifiers into a digest, not rendering to HTML.
	}
	sum := h.Sum(nil)
	return hex.EncodeToString(sum)
}

// precomputeFingerprint builds the cheap fingerprint from the generation
// counter and other fast-to-read values, avoiding the 8+ DB queries that a
// full response build would require.
func (d *Deps) precomputeFingerprint(
	ctx context.Context,
	groupFilter string,
) string {
	gen := d.Cache.Generation.Load()
	scopeKey := terminalScopeKey(principalActorID(ctx))

	processedJobs := uint64(0)
	if d.ProcessedJobs != nil {
		processedJobs = d.ProcessedJobs.Load()
	}

	retentionError := ""
	if d.RetentionTracker != nil {
		d.RetentionTracker.Mu.RLock()
		retentionError = strings.TrimSpace(d.RetentionTracker.LastErr)
		d.RetentionTracker.Mu.RUnlock()
	}

	parts := []string{
		strings.TrimSpace(groupFilter),
		scopeKey,
		strconv.FormatUint(gen, 10),
		strconv.FormatUint(processedJobs, 10),
		retentionError,
	}
	return Fingerprint(parts)
}

// groupFilter extracts the group_id query parameter from the request.
func groupFilter(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("group_id"))
}

// HandleStatusAggregate returns an http.HandlerFunc for GET /status/aggregate.
// It is used as a closure so that the per-handler ETag cache is allocated once
// at registration time rather than per-request.
func (d *Deps) HandleStatusAggregate() http.HandlerFunc {
	cache := newETagCache()

	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status/aggregate" {
			servicehttp.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		gf := groupFilter(r)
		caller := shared.SourceQueryCaller(r, "status.aggregate")
		scopeKey := terminalScopeKey(principalActorID(r.Context()))
		cacheKey := gf + "|" + scopeKey

		var allAssets []assets.Asset
		allAssetsLoaded := false
		loadAllAssets := func() []assets.Asset {
			if allAssetsLoaded {
				return allAssets
			}
			allAssets = d.listAssets()
			allAssetsLoaded = true
			return allAssets
		}

		w.Header().Set("Cache-Control", "private, max-age=30")
		ifNoneMatch := strings.TrimSpace(r.Header.Get("If-None-Match"))
		fingerprint := ""
		if ifNoneMatch != "" && cache.hasKey(cacheKey) {
			fingerprint = d.precomputeFingerprint(r.Context(), gf)
			if cachedETag, ok := cache.match(cacheKey, fingerprint); ok {
				w.Header().Set("ETag", cachedETag)
				if ETagMatches(ifNoneMatch, cachedETag) {
					w.WriteHeader(http.StatusNotModified)
					return
				}
			}
		}

		response := d.buildResponseDeduped(r.Context(), gf, caller, loadAllAssets())
		etag := ETag(response)

		if etag != "" {
			w.Header().Set("ETag", etag)
			if fingerprint == "" && ifNoneMatch != "" {
				fingerprint = d.precomputeFingerprint(r.Context(), gf)
			}
			if fingerprint != "" {
				cache.store(cacheKey, fingerprint, etag)
			}
			if ETagMatches(r.Header.Get("If-None-Match"), etag) {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}

		servicehttp.WriteJSON(w, http.StatusOK, response)
	}
}

// HandleStatusAggregateLive returns an http.HandlerFunc for GET /status/aggregate/live.
func (d *Deps) HandleStatusAggregateLive() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status/aggregate/live" {
			servicehttp.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		gf := groupFilter(r)
		servicehttp.WriteJSON(w, http.StatusOK, d.BuildLiveResponseDeduped(r.Context(), gf))
	}
}

// BuildLiveResponse builds the lightweight live status response.
func (d *Deps) BuildLiveResponse(ctx context.Context, gf string) LiveResponse {
	return d.BuildLiveResponseDeduped(ctx, gf)
}

// BuildLiveResponseDeduped deduplicates concurrent live response builds for
// the same group/asset fingerprint using a singleflight group.
func (d *Deps) BuildLiveResponseDeduped(ctx context.Context, gf string) LiveResponse {
	assetsAll := d.listAssets()
	key := "live:" + strings.TrimSpace(gf) + ":" + CanonicalAssetFingerprint(assetsAll)
	value, _, _ := d.Cache.LiveBuildGroup.Do(key, func() (any, error) {
		return d.buildLiveResponseWithAssets(ctx, gf, assetsAll), nil
	})
	response, ok := value.(LiveResponse)
	if !ok {
		return d.buildLiveResponseWithAssets(ctx, gf, assetsAll)
	}
	return response
}

func (d *Deps) buildLiveResponseWithAssets(
	ctx context.Context,
	gf string,
	assetsAll []assets.Asset,
) LiveResponse {
	now := time.Now().UTC()
	assetsFiltered := FilterAssetsByGroup(assetsAll, gf)
	endpoints := d.collectEndpointResults(ctx)
	telemetryOverview := d.buildTelemetryOverview(assetsFiltered, now)
	servicesUp, servicesTotal := d.webServiceSummary(assetsFiltered)

	return LiveResponse{
		Timestamp: now,
		Demo:      d.DemoMode,
		Summary: LiveSummary{
			ServicesUp:      servicesUp,
			ServicesTotal:   servicesTotal,
			AssetCount:      len(assetsFiltered),
			StaleAssetCount: CountStaleAssets(assetsFiltered, now),
		},
		Endpoints:         endpoints,
		Assets:            assetsFiltered,
		TelemetryOverview: telemetryOverview,
	}
}

// BuildResponse builds the full status aggregate response.
func (d *Deps) BuildResponse(ctx context.Context, gf string) Response {
	return d.buildResponseDeduped(ctx, gf, "status.aggregate", d.listAssets())
}

func (d *Deps) buildResponseDeduped(
	ctx context.Context,
	gf string,
	caller string,
	allAssets []assets.Asset,
) Response {
	if allAssets == nil {
		allAssets = d.listAssets()
	}
	processedJobs := uint64(0)
	if d.ProcessedJobs != nil {
		processedJobs = d.ProcessedJobs.Load()
	}
	scopeKey := terminalScopeKey(principalActorID(ctx))
	key := strings.Join([]string{
		"full",
		strings.TrimSpace(gf),
		scopeKey,
		shared.NormalizeSourceQueryCaller(caller, "status.aggregate"),
		CanonicalAssetFingerprint(allAssets),
		strconv.FormatUint(processedJobs, 10),
	}, ":")
	value, _, _ := d.Cache.FullBuildGroup.Do(key, func() (any, error) {
		return d.buildResponseWithAssets(ctx, gf, caller, allAssets), nil
	})
	response, ok := value.(Response)
	if !ok {
		return d.buildResponseWithAssets(ctx, gf, caller, allAssets)
	}
	return response
}

func (d *Deps) buildResponseWithAssets(
	ctx context.Context,
	gf string,
	caller string,
	allAssets []assets.Asset,
) Response {
	live := d.buildLiveResponseWithAssets(ctx, gf, allAssets)
	collections := d.collectAggregateCollections(ctx, gf, caller, allAssets)

	processedJobs := uint64(0)
	if d.ProcessedJobs != nil {
		processedJobs = d.ProcessedJobs.Load()
	}

	retentionError := ""
	if d.RetentionTracker != nil {
		d.RetentionTracker.Mu.RLock()
		retentionError = strings.TrimSpace(d.RetentionTracker.LastErr)
		d.RetentionTracker.Mu.RUnlock()
	}

	return Response{
		Timestamp: live.Timestamp,
		Demo:      d.DemoMode,
		Summary: Summary{
			ServicesUp:      live.Summary.ServicesUp,
			ServicesTotal:   live.Summary.ServicesTotal,
			ConnectorCount:  len(collections.connectors),
			GroupCount:      len(collections.groups),
			AssetCount:      len(live.Assets),
			SessionCount:    len(collections.sessions),
			AuditCount:      len(collections.recentAudit),
			ProcessedJobs:   processedJobs,
			ActionRunCount:  len(collections.actionRuns),
			UpdateRunCount:  len(collections.updateRuns),
			DeadLetterCount: collections.deadLetterTotal,
			StaleAssetCount: live.Summary.StaleAssetCount,
			RetentionError:  retentionError,
		},
		Endpoints:           live.Endpoints,
		Connectors:          collections.connectors,
		Groups:              collections.groups,
		Assets:              live.Assets,
		TelemetryOverview:   live.TelemetryOverview,
		RecentLogs:          collections.recentLogs,
		LogSources:          collections.logSources,
		ActionRuns:          collections.actionRuns,
		UpdatePlans:         collections.updatePlans,
		UpdateRuns:          collections.updateRuns,
		DeadLetters:         collections.deadLetterEvents,
		DeadLetterAnalytics: collections.deadLetterStats,
		Sessions:            collections.sessions,
		RecentCommands:      collections.recentCommands,
		RecentAudit:         collections.recentAudit,
		Canonical:           d.canonicalPayload(live.Assets),
	}
}

// RegisterRoutes registers the status aggregate HTTP routes on mux.
func RegisterRoutes(mux *http.ServeMux, d *Deps, wrapAuth func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("/status/aggregate", wrapAuth(d.HandleStatusAggregate()))
	mux.HandleFunc("/status/aggregate/live", wrapAuth(d.HandleStatusAggregateLive()))
}
