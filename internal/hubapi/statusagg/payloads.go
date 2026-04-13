package statusagg

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/groups"
	groupfeatures "github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/model"
	"github.com/labtether/labtether/internal/modelregistry"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/labtether/labtether/internal/updates"
)

// --- Response payload types ---

// LiveSummary is the lightweight summary included in the live status response.
type LiveSummary struct {
	ServicesUp      int `json:"servicesUp"`
	ServicesTotal   int `json:"servicesTotal"`
	AssetCount      int `json:"assetCount"`
	StaleAssetCount int `json:"staleAssetCount"`
}

// LiveResponse is the lightweight status response used by /status/aggregate/live.
type LiveResponse struct {
	Timestamp         time.Time                       `json:"timestamp"`
	Demo              bool                            `json:"demo"`
	Summary           LiveSummary                     `json:"summary"`
	Endpoints         []EndpointResult                `json:"endpoints"`
	Assets            []assets.Asset                  `json:"assets"`
	TelemetryOverview []shared.AssetTelemetryOverview `json:"telemetryOverview"`
}

// Summary is the full aggregated summary included in the full status response.
type Summary struct {
	ServicesUp      int    `json:"servicesUp"`
	ServicesTotal   int    `json:"servicesTotal"`
	ConnectorCount  int    `json:"connectorCount"`
	GroupCount      int    `json:"groupCount"`
	AssetCount      int    `json:"assetCount"`
	SessionCount    int    `json:"sessionCount"`
	AuditCount      int    `json:"auditCount"`
	ProcessedJobs   uint64 `json:"processedJobs"`
	ActionRunCount  int    `json:"actionRunCount"`
	UpdateRunCount  int    `json:"updateRunCount"`
	DeadLetterCount int    `json:"deadLetterCount"`
	StaleAssetCount int    `json:"staleAssetCount"`
	RetentionError  string `json:"retentionError,omitempty"`
}

// Response is the full status aggregate response.
type Response struct {
	Timestamp           time.Time                              `json:"timestamp"`
	Demo                bool                                   `json:"demo"`
	Summary             Summary                                `json:"summary"`
	Endpoints           []EndpointResult                       `json:"endpoints"`
	Connectors          []connectorsdk.Descriptor              `json:"connectors"`
	Groups              []groups.Group                         `json:"groups"`
	Assets              []assets.Asset                         `json:"assets"`
	TelemetryOverview   []shared.AssetTelemetryOverview        `json:"telemetryOverview"`
	RecentLogs          []logs.Event                           `json:"recentLogs"`
	LogSources          []logs.SourceSummary                   `json:"logSources"`
	GroupReliability    []groupfeatures.GroupReliabilityRecord `json:"groupReliability"`
	ActionRuns          []actions.Run                          `json:"actionRuns"`
	UpdatePlans         []updates.Plan                         `json:"updatePlans"`
	UpdateRuns          []updates.Run                          `json:"updateRuns"`
	DeadLetters         []shared.DeadLetterEventResponse       `json:"deadLetters"`
	DeadLetterAnalytics shared.DeadLetterAnalyticsResponse     `json:"deadLetterAnalytics"`
	Sessions            []terminal.Session                     `json:"sessions"`
	RecentCommands      []terminal.Command                     `json:"recentCommands"`
	RecentAudit         []audit.Event                          `json:"recentAudit"`
	Canonical           CanonicalPayload                       `json:"canonical"`
}

// CanonicalPayload is the canonical model snapshot included in the full status
// aggregate response.
type CanonicalPayload struct {
	Registry         modelregistry.CanonicalRegistry  `json:"registry"`
	Providers        []model.ProviderInstance         `json:"providers"`
	CapabilitySets   []model.CapabilitySet            `json:"capabilitySets"`
	TemplateBindings map[string]model.TemplateBinding `json:"templateBindings"`
	Reconciliation   []model.ReconciliationResult     `json:"reconciliation"`
}

// --- Canonical payload helpers ---

// canonicalPayload builds the CanonicalPayload from the canonical store,
// using a watermark-keyed cache to avoid redundant DB round-trips.
func (d *Deps) canonicalPayload(assetList []assets.Asset) CanonicalPayload {
	registry := modelregistry.Snapshot()
	payload := CanonicalPayload{
		Registry:         registry,
		Providers:        []model.ProviderInstance{},
		CapabilitySets:   []model.CapabilitySet{},
		TemplateBindings: map[string]model.TemplateBinding{},
		Reconciliation:   []model.ReconciliationResult{},
	}
	if d.CanonicalStore == nil {
		return payload
	}

	cacheKey := ""
	if reader, ok := d.CanonicalStore.(CanonicalWatermarkReader); ok {
		if watermark, err := reader.CanonicalStatusWatermark(); err != nil {
			log.Printf("status aggregate: failed to read canonical watermark: %v", err)
		} else {
			cacheKey = canonicalPayloadCacheKey(watermark, assetList)
			if cached, ok := d.canonicalPayloadCacheLookup(cacheKey); ok {
				cached.Registry = registry
				return cached
			}
		}
	}

	providers, err := d.CanonicalStore.ListProviderInstances(500)
	if err != nil {
		log.Printf("status aggregate: failed to list canonical providers: %v", err)
	} else if providers != nil {
		payload.Providers = providers
	}

	capabilitySets, err := d.CanonicalStore.ListCapabilitySets(2000)
	if err != nil {
		log.Printf("status aggregate: failed to list canonical capability sets: %v", err)
	} else if capabilitySets != nil {
		payload.CapabilitySets = capabilitySets
	}

	assetIDs := make([]string, 0, len(assetList))
	for _, assetEntry := range assetList {
		assetID := strings.TrimSpace(assetEntry.ID)
		if assetID == "" {
			continue
		}
		assetIDs = append(assetIDs, assetID)
	}
	templateBindings, err := d.CanonicalStore.ListTemplateBindings(assetIDs)
	if err != nil {
		log.Printf("status aggregate: failed to list canonical template bindings: %v", err)
	} else {
		for _, binding := range templateBindings {
			payload.TemplateBindings[binding.ResourceID] = binding
		}
	}

	reconciliation, err := d.CanonicalStore.ListReconciliationResults("", 300)
	if err != nil {
		log.Printf("status aggregate: failed to list canonical reconciliation results: %v", err)
	} else if reconciliation != nil {
		payload.Reconciliation = reconciliation
	}

	if cacheKey != "" {
		d.canonicalPayloadCacheStore(cacheKey, payload)
	}

	return payload
}

func canonicalPayloadCacheKey(watermark time.Time, assetList []assets.Asset) string {
	return strconv.FormatInt(watermark.UTC().UnixNano(), 10) + ":" + CanonicalAssetFingerprint(assetList)
}

func (d *Deps) canonicalPayloadCacheLookup(key string) (CanonicalPayload, bool) {
	if strings.TrimSpace(key) == "" {
		return CanonicalPayload{}, false
	}
	d.Cache.CanonicalCacheMu.RLock()
	defer d.Cache.CanonicalCacheMu.RUnlock()

	for _, entry := range d.Cache.CanonicalCache {
		if entry.Key == key {
			return entry.Payload, true
		}
	}
	return CanonicalPayload{}, false
}

func (d *Deps) canonicalPayloadCacheStore(key string, payload CanonicalPayload) {
	if strings.TrimSpace(key) == "" {
		return
	}
	d.Cache.CanonicalCacheMu.Lock()
	d.Cache.CanonicalCache[d.Cache.CanonicalCacheIdx] = CanonicalPayloadCacheEntry{
		Key:     key,
		Payload: payload,
	}
	d.Cache.CanonicalCacheIdx = (d.Cache.CanonicalCacheIdx + 1) % len(d.Cache.CanonicalCache)
	d.Cache.CanonicalCacheMu.Unlock()
}

// --- Asset fingerprinting helpers ---

// CanonicalAssetFingerprint returns a stable hash of all asset IDs in the
// provided list. It is used as part of cache keys.
func CanonicalAssetFingerprint(assetList []assets.Asset) string {
	assetIDs := CanonicalAssetIDs(assetList)
	if len(assetIDs) == 0 {
		return "none"
	}
	h := sha256.New()
	for _, assetID := range assetIDs {
		_, _ = io.WriteString(h, assetID) // #nosec G705 -- Hashing asset IDs into a digest, not rendering to HTML.
		_, _ = h.Write([]byte{0})
	}
	sum := h.Sum(nil)
	return hex.EncodeToString(sum)
}

// CanonicalAssetIDs returns a sorted, deduplicated list of asset IDs.
func CanonicalAssetIDs(assetList []assets.Asset) []string {
	if len(assetList) == 0 {
		return nil
	}
	assetIDs := make([]string, 0, len(assetList))
	seen := make(map[string]struct{}, len(assetList))
	for _, assetEntry := range assetList {
		assetID := strings.TrimSpace(assetEntry.ID)
		if assetID == "" {
			continue
		}
		if _, ok := seen[assetID]; ok {
			continue
		}
		seen[assetID] = struct{}{}
		assetIDs = append(assetIDs, assetID)
	}
	sort.Strings(assetIDs)
	return assetIDs
}

// --- ETag computation ---

// ETag computes a stable ETag for the full status aggregate response.
func ETag(response Response) string {
	h := sha256.New()
	hashWriteString(h, "status-aggregate-etag-v2")
	hashWriteString(h, response.Timestamp.UTC().Format(time.RFC3339Nano))

	hashWriteInt(h, response.Summary.ServicesUp)
	hashWriteInt(h, response.Summary.ServicesTotal)
	hashWriteInt(h, response.Summary.ConnectorCount)
	hashWriteInt(h, response.Summary.GroupCount)
	hashWriteInt(h, response.Summary.AssetCount)
	hashWriteInt(h, response.Summary.SessionCount)
	hashWriteInt(h, response.Summary.AuditCount)
	hashWriteUint64(h, response.Summary.ProcessedJobs)
	hashWriteInt(h, response.Summary.ActionRunCount)
	hashWriteInt(h, response.Summary.UpdateRunCount)
	hashWriteInt(h, response.Summary.DeadLetterCount)
	hashWriteInt(h, response.Summary.StaleAssetCount)
	hashWriteString(h, response.Summary.RetentionError)

	for _, endpoint := range response.Endpoints {
		hashWriteString(h, endpoint.Name)
		hashWriteString(h, endpoint.URL)
		hashWriteBool(h, endpoint.OK)
		hashWriteString(h, endpoint.Status)
		hashWriteInt(h, endpoint.Code)
		hashWriteInt64(h, endpoint.LatencyMs)
		hashWriteString(h, endpoint.Error)
	}

	for _, connector := range response.Connectors {
		hashWriteString(h, connector.ID)
		hashWriteString(h, connector.DisplayName)
		hashWriteBool(h, connector.Capabilities.DiscoverAssets)
		hashWriteBool(h, connector.Capabilities.CollectMetrics)
		hashWriteBool(h, connector.Capabilities.CollectEvents)
		hashWriteBool(h, connector.Capabilities.ExecuteActions)
	}

	for _, groupEntry := range response.Groups {
		hashWriteString(h, groupEntry.ID)
		hashWriteString(h, groupEntry.Name)
		hashWriteString(h, groupEntry.Slug)
		hashWriteTime(h, groupEntry.UpdatedAt)
	}

	for _, assetEntry := range response.Assets {
		hashWriteString(h, assetEntry.ID)
		hashWriteString(h, assetEntry.Type)
		hashWriteString(h, assetEntry.Name)
		hashWriteString(h, assetEntry.Source)
		hashWriteString(h, assetEntry.GroupID)
		hashWriteString(h, assetEntry.Status)
		hashWriteString(h, assetEntry.Platform)
		hashWriteString(h, assetEntry.ResourceClass)
		hashWriteString(h, assetEntry.ResourceKind)
		hashWriteTime(h, assetEntry.UpdatedAt)
		hashWriteTime(h, assetEntry.LastSeenAt)
	}

	for _, telemetryEntry := range response.TelemetryOverview {
		hashWriteString(h, telemetryEntry.AssetID)
		hashWriteString(h, telemetryEntry.Status)
		hashWriteTime(h, telemetryEntry.LastSeenAt)
		hashWriteFloatPtr(h, telemetryEntry.Metrics.CPUUsedPercent)
		hashWriteFloatPtr(h, telemetryEntry.Metrics.MemoryUsedPercent)
		hashWriteFloatPtr(h, telemetryEntry.Metrics.DiskUsedPercent)
		hashWriteFloatPtr(h, telemetryEntry.Metrics.TemperatureCelsius)
		hashWriteFloatPtr(h, telemetryEntry.Metrics.NetworkRXBytesPerSec)
		hashWriteFloatPtr(h, telemetryEntry.Metrics.NetworkTXBytesPerSec)
	}

	for _, event := range response.RecentLogs {
		hashWriteString(h, event.ID)
		hashWriteString(h, event.AssetID)
		hashWriteString(h, event.Source)
		hashWriteString(h, event.Level)
		hashWriteString(h, event.Message)
		hashWriteTime(h, event.Timestamp)
	}

	for _, source := range response.LogSources {
		hashWriteString(h, source.Source)
		hashWriteInt(h, source.Count)
		hashWriteTime(h, source.LastSeenAt)
	}

	for _, run := range response.ActionRuns {
		hashWriteString(h, run.ID)
		hashWriteString(h, run.Status)
		hashWriteTime(h, run.UpdatedAt)
	}

	for _, plan := range response.UpdatePlans {
		hashWriteString(h, plan.ID)
		hashWriteTime(h, plan.UpdatedAt)
	}

	for _, run := range response.UpdateRuns {
		hashWriteString(h, run.ID)
		hashWriteString(h, run.Status)
		hashWriteTime(h, run.UpdatedAt)
	}

	for _, deadLetter := range response.DeadLetters {
		hashWriteString(h, deadLetter.ID)
		hashWriteString(h, deadLetter.Component)
		hashWriteString(h, deadLetter.Subject)
		hashWriteUint64(h, deadLetter.Deliveries)
		hashWriteString(h, deadLetter.Error)
		hashWriteTime(h, deadLetter.CreatedAt)
	}

	hashWriteString(h, response.DeadLetterAnalytics.Window)
	hashWriteString(h, response.DeadLetterAnalytics.Bucket)
	hashWriteInt(h, response.DeadLetterAnalytics.Total)
	hashWriteString(h, strconv.FormatFloat(response.DeadLetterAnalytics.RatePerHour, 'g', -1, 64))
	hashWriteString(h, strconv.FormatFloat(response.DeadLetterAnalytics.RatePerDay, 'g', -1, 64))
	for _, point := range response.DeadLetterAnalytics.Trend {
		hashWriteTime(h, point.Start)
		hashWriteTime(h, point.End)
		hashWriteInt(h, point.Count)
	}
	for _, entry := range response.DeadLetterAnalytics.TopComponents {
		hashWriteString(h, entry.Key)
		hashWriteInt(h, entry.Count)
	}
	for _, entry := range response.DeadLetterAnalytics.TopSubjects {
		hashWriteString(h, entry.Key)
		hashWriteInt(h, entry.Count)
	}
	for _, entry := range response.DeadLetterAnalytics.TopErrorClasses {
		hashWriteString(h, entry.Key)
		hashWriteInt(h, entry.Count)
	}

	for _, session := range response.Sessions {
		hashWriteString(h, session.ID)
		hashWriteString(h, session.Status)
		hashWriteTime(h, session.LastActionAt)
	}

	for _, command := range response.RecentCommands {
		hashWriteString(h, command.ID)
		hashWriteString(h, command.Status)
		hashWriteTime(h, command.UpdatedAt)
	}

	for _, auditEvent := range response.RecentAudit {
		hashWriteString(h, auditEvent.ID)
		hashWriteString(h, auditEvent.Type)
		hashWriteTime(h, auditEvent.Timestamp)
	}

	for _, capability := range response.Canonical.Registry.Capabilities {
		hashWriteString(h, capability.ID)
		hashWriteString(h, string(capability.Scope))
		hashWriteString(h, string(capability.Stability))
	}
	for _, operation := range response.Canonical.Registry.Operations {
		hashWriteString(h, operation.ID)
	}
	for _, metric := range response.Canonical.Registry.Metrics {
		hashWriteString(h, metric.ID)
		hashWriteString(h, metric.Unit)
	}
	for _, eventDescriptor := range response.Canonical.Registry.Events {
		hashWriteString(h, eventDescriptor.ID)
		hashWriteString(h, string(eventDescriptor.Kind))
	}
	for _, template := range response.Canonical.Registry.Templates {
		hashWriteString(h, template.ID)
		hashWriteString(h, template.Version)
	}

	for _, provider := range response.Canonical.Providers {
		hashWriteString(h, provider.ID)
		hashWriteString(h, provider.Provider)
		hashWriteString(h, string(provider.Status))
		hashWriteTime(h, provider.UpdatedAt)
	}

	for _, capabilitySet := range response.Canonical.CapabilitySets {
		hashWriteString(h, capabilitySet.SubjectType)
		hashWriteString(h, capabilitySet.SubjectID)
		hashWriteTime(h, capabilitySet.UpdatedAt)
	}

	templateKeys := make([]string, 0, len(response.Canonical.TemplateBindings))
	for key := range response.Canonical.TemplateBindings {
		templateKeys = append(templateKeys, key)
	}
	sort.Strings(templateKeys)
	for _, key := range templateKeys {
		binding := response.Canonical.TemplateBindings[key]
		hashWriteString(h, binding.ResourceID)
		hashWriteString(h, binding.TemplateID)
		hashWriteTime(h, binding.UpdatedAt)
		for _, tab := range binding.Tabs {
			hashWriteString(h, tab)
		}
		for _, operation := range binding.Operations {
			hashWriteString(h, operation)
		}
	}

	for _, reconciliation := range response.Canonical.Reconciliation {
		hashWriteString(h, reconciliation.ProviderInstanceID)
		hashWriteInt(h, reconciliation.CreatedCount)
		hashWriteInt(h, reconciliation.UpdatedCount)
		hashWriteInt(h, reconciliation.StaleCount)
		hashWriteInt(h, reconciliation.ErrorCount)
		hashWriteTime(h, reconciliation.FinishedAt)
	}

	sum := h.Sum(nil)
	return hex.EncodeToString(sum)
}

// ETagMatches reports whether the If-None-Match header value matches the etag.
func ETagMatches(headerValue, etag string) bool {
	etag = strings.TrimSpace(etag)
	if etag == "" {
		return false
	}
	for _, part := range strings.Split(headerValue, ",") {
		token := strings.Trim(strings.TrimSpace(part), `"`)
		if token == "" {
			continue
		}
		if token == "*" || token == etag {
			return true
		}
	}
	return false
}

// --- Hash helpers ---

func hashWriteString(h hash.Hash, value string) {
	_, _ = io.WriteString(h, strings.TrimSpace(value)) // #nosec G705 -- Hashing bounded strings into a digest, not rendering to HTML.
	_, _ = h.Write([]byte{0})
}

func hashWriteBool(h hash.Hash, value bool) {
	if value {
		hashWriteString(h, "1")
		return
	}
	hashWriteString(h, "0")
}

func hashWriteInt(h hash.Hash, value int) {
	hashWriteString(h, strconv.Itoa(value))
}

func hashWriteInt64(h hash.Hash, value int64) {
	hashWriteString(h, strconv.FormatInt(value, 10))
}

func hashWriteUint64(h hash.Hash, value uint64) {
	hashWriteString(h, strconv.FormatUint(value, 10))
}

func hashWriteTime(h hash.Hash, value time.Time) {
	hashWriteString(h, strconv.FormatInt(value.UTC().UnixNano(), 10))
}

func hashWriteFloatPtr(h hash.Hash, value *float64) {
	if value == nil {
		hashWriteString(h, "")
		return
	}
	hashWriteString(h, strconv.FormatFloat(*value, 'g', -1, 64))
}
