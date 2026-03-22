package webservice

import (
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/agentcore"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/persistence"
	"golang.org/x/sync/singleflight"
)

const defaultHostTTL = 5 * time.Minute

const (
	labtetherServiceKey      = "labtether"
	labtetherConsole         = "console"
	labtetherAPI             = "api"
	serviceHealthWindow      = 24 * time.Hour
	serviceHealthWindowLabel = "24h"
	serviceHealthMaxSamples  = 2048
	serviceHealthRecentLimit = 24
	serviceHealthCoalesce    = time.Minute
)

var ErrStoreUnavailable = errors.New("web service store unavailable")

type hostEntry struct {
	services       []agentmgr.DiscoveredWebService
	discovery      *agentmgr.WebServiceDiscoveryStats
	lastSeen       time.Time
	disconnectedAt *time.Time
}

type DiscoveryStatsSnapshot struct {
	HostAssetID string                            `json:"host_asset_id"`
	LastSeen    time.Time                         `json:"last_seen"`
	Discovery   agentmgr.WebServiceDiscoveryStats `json:"discovery"`
}

type serviceHealthSample struct {
	at         time.Time
	status     string
	responseMs int
}

type serviceHealthHistory struct {
	samples []serviceHealthSample
}

type storeSnapshot struct {
	manuals   []persistence.WebServiceManual
	overrides map[string]persistence.WebServiceOverride
}

// Coordinator aggregates web service reports from all connected agents.
// It follows the same hub-side aggregation pattern as the Docker coordinator.
type Coordinator struct {
	mu            sync.RWMutex
	hosts         map[string]*hostEntry
	serviceHealth map[string]*serviceHealthHistory
	hostTTL       time.Duration
	store         persistence.WebServiceStore
	nowFn         func() time.Time
	storeCacheMu  sync.RWMutex
	storeCache    *storeSnapshot
	storeLoad     singleflight.Group
}

// NewCoordinator creates a new WebService coordinator.
func NewCoordinator(store ...persistence.WebServiceStore) *Coordinator {
	var cfgStore persistence.WebServiceStore
	if len(store) > 0 {
		cfgStore = store[0]
	}
	return &Coordinator{
		hosts:         make(map[string]*hostEntry),
		serviceHealth: make(map[string]*serviceHealthHistory),
		hostTTL:       defaultHostTTL,
		store:         cfgStore,
		nowFn:         time.Now,
	}
}

// HandleReport processes a webservice.report message from an agent.
// Each report fully replaces the previous service list for that host.
func (c *Coordinator) HandleReport(agentID string, msg agentmgr.Message) {
	var data agentmgr.WebServiceReportData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("webservice-coordinator: invalid report from %s: %v", agentID, err)
		return
	}

	enrichServicesFromRegistry(data.Services)

	reportHostID := normalizeServiceHostID(data.HostAssetID, agentID)
	now := c.now()
	services := make([]agentmgr.DiscoveredWebService, 0, len(data.Services))
	for _, raw := range data.Services {
		svc := cloneDiscoveredService(raw)
		svc.HostAssetID = normalizeServiceHostID(svc.HostAssetID, reportHostID)
		c.recordServiceHealthSampleLocked(svc.HostAssetID, svc.ID, svc.Status, svc.ResponseMs, now)
		services = append(services, svc)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.hosts[agentID] = &hostEntry{
		services:  services,
		discovery: cloneWebServiceDiscoveryStats(data.Discovery),
		lastSeen:  now,
	}
	c.pruneServiceHealthLocked(now)
}

func (c *Coordinator) DiscoveryStats(hostFilter string) []DiscoveryStatsSnapshot {
	trimmed := strings.TrimSpace(hostFilter)

	c.mu.RLock()
	defer c.mu.RUnlock()

	snapshots := make([]DiscoveryStatsSnapshot, 0, len(c.hosts))
	for hostID, entry := range c.hosts {
		if trimmed != "" && hostID != trimmed {
			continue
		}
		if entry == nil || entry.discovery == nil {
			continue
		}
		cloned := cloneWebServiceDiscoveryStats(entry.discovery)
		if cloned == nil {
			continue
		}
		snapshots = append(snapshots, DiscoveryStatsSnapshot{
			HostAssetID: hostID,
			LastSeen:    entry.lastSeen,
			Discovery:   *cloned,
		})
	}

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].HostAssetID < snapshots[j].HostAssetID
	})

	return snapshots
}

// ListAll returns all discovered web services across all hosts,
// sorted deterministically by HostAssetID + ID to prevent UI reshuffling.
func (c *Coordinator) ListAll() []agentmgr.DiscoveredWebService {
	all := c.snapshotAllHostServices()
	all = c.mergeWithManualAndOverrides(all, "")

	sort.Slice(all, func(i, j int) bool {
		if all[i].HostAssetID != all[j].HostAssetID {
			return all[i].HostAssetID < all[j].HostAssetID
		}
		return all[i].ID < all[j].ID
	})

	return all
}

// ListByHost returns discovered web services for a specific host.
func (c *Coordinator) ListByHost(hostID string) []agentmgr.DiscoveredWebService {
	trimmed := strings.TrimSpace(hostID)
	if trimmed == "" {
		return nil
	}
	result := c.snapshotHostServices(trimmed)
	result = c.mergeWithManualAndOverrides(result, trimmed)
	return result
}

// AttachHealthSummaries populates per-service rolling uptime and status history.
func (c *Coordinator) AttachHealthSummaries(services []agentmgr.DiscoveredWebService) {
	if len(services) == 0 {
		return
	}
	now := c.now()

	c.mu.RLock()
	defer c.mu.RUnlock()

	for index := range services {
		key := serviceOverrideKey(services[index].HostAssetID, services[index].ID)
		services[index].Health = buildServiceHealthSummary(c.serviceHealth[key], now)
	}
}

// Categories returns a sorted list of unique categories from all active services.
func (c *Coordinator) Categories() []string {
	services := c.ListAll()
	seen := make(map[string]struct{})
	for _, svc := range services {
		if isHiddenService(svc) {
			continue
		}
		if svc.Category != "" {
			seen[svc.Category] = struct{}{}
		}
	}

	cats := make([]string, 0, len(seen))
	for cat := range seen {
		cats = append(cats, cat)
	}
	sort.Strings(cats)
	return cats
}

type serviceSummaryEntry struct {
	hostAssetID string
	serviceID   string
	status      string
	serviceKey  string
	component   string
	hidden      bool
}

// SummaryByHosts returns visible service up/total counts for one or more hosts.
// An empty host filter means all hosts.
func (c *Coordinator) SummaryByHosts(hostFilter map[string]struct{}) (up int, total int) {
	baseServices, hostHasConsole := c.snapshotServiceSummaryEntries(hostFilter)

	overrideMap := map[string]persistence.WebServiceOverride{}
	manuals := []persistence.WebServiceManual{}
	snapshot, err := c.loadStoreSnapshot()
	if err != nil {
		log.Printf("webservice-coordinator: failed to load service store snapshot for summary: %v", err)
	} else if snapshot != nil {
		overrideMap = snapshot.overrides
		manuals = snapshot.manuals
	}

	seen := make(map[string]struct{}, len(baseServices))
	for _, svc := range baseServices {
		key := serviceOverrideKey(svc.hostAssetID, svc.serviceID)
		seen[key] = struct{}{}

		hidden := svc.hidden
		if svc.serviceKey == labtetherServiceKey {
			switch svc.component {
			case labtetherConsole:
				hidden = false
			case labtetherAPI:
				hidden = hostHasConsole[svc.hostAssetID]
			}
		}
		if override, ok := overrideMap[key]; ok {
			hidden = override.Hidden
		}
		if hidden {
			continue
		}

		total++
		if strings.EqualFold(svc.status, "up") {
			up++
		}
	}

	for _, manual := range manuals {
		hostID := strings.TrimSpace(manual.HostAssetID)
		if !summaryHostAllowed(hostID, hostFilter) {
			continue
		}
		manualID := strings.TrimSpace(manual.ID)
		if manualID == "" {
			continue
		}
		key := serviceOverrideKey(hostID, manualID)
		if _, exists := seen[key]; exists {
			continue
		}

		hidden := false
		if override, ok := overrideMap[key]; ok {
			hidden = override.Hidden
		}
		if hidden {
			continue
		}
		total++
	}

	return up, total
}

// MarkHostDisconnected sets all services for a host to "unknown" status
// and records the disconnection time. The host entry is retained until
// CleanExpired removes it after the TTL elapses.
func (c *Coordinator) MarkHostDisconnected(hostID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.hosts[hostID]
	if !ok {
		return
	}

	now := c.now()
	entry.disconnectedAt = &now
	for i := range entry.services {
		entry.services[i].Status = "unknown"
		c.recordServiceHealthSampleLocked(entry.services[i].HostAssetID, entry.services[i].ID, "unknown", 0, now)
	}
	c.pruneServiceHealthLocked(now)
}

// RemoveHost immediately removes all cached discovered services for a host.
// This is used when the owning asset is deleted and services should disappear
// from the UI without waiting for disconnect TTL expiry.
func (c *Coordinator) RemoveHost(hostID string) {
	trimmed := strings.TrimSpace(hostID)
	if trimmed == "" {
		return
	}

	c.mu.Lock()
	c.removeServiceHealthForHostLocked(trimmed)
	delete(c.hosts, trimmed)
	c.mu.Unlock()

	c.removePersistedHostEntries(trimmed)
	c.invalidateStoreSnapshot()
}

func (c *Coordinator) removePersistedHostEntries(hostID string) {
	if c.store == nil {
		return
	}

	if err := c.store.PromoteManualServicesToStandalone(hostID); err != nil {
		log.Printf("webservice-coordinator: failed to promote manual services to standalone for host %s: %v", hostID, err)
		// Continue to override cleanup — on asset deletion, ON DELETE CASCADE
		// already handles overrides at the DB level, so this loop is a safety net.
		// Returning early here would orphan overrides if RemoveHost is called
		// without an actual asset deletion (e.g., coordinator cleanup).
	}

	overrides, err := c.store.ListWebServiceOverrides(hostID)
	if err != nil {
		log.Printf("webservice-coordinator: failed to list overrides for host removal %s: %v", hostID, err)
		return
	}
	for _, override := range overrides {
		serviceID := strings.TrimSpace(override.ServiceID)
		if serviceID == "" {
			continue
		}
		if err := c.store.DeleteWebServiceOverride(hostID, serviceID); err != nil && !errors.Is(err, persistence.ErrNotFound) {
			log.Printf("webservice-coordinator: failed to delete override %s during host removal %s: %v", serviceID, hostID, err)
		}
	}
}

// ClearAll removes all cached hosts, services, and health history.
// Used after an admin data reset to ensure the in-memory state matches the DB.
func (c *Coordinator) ClearAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.hosts = make(map[string]*hostEntry)
	c.serviceHealth = make(map[string]*serviceHealthHistory)
	c.invalidateStoreSnapshot()
}

// CleanExpired removes hosts that have been disconnected longer than the TTL.
func (c *Coordinator) CleanExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	for hostID, entry := range c.hosts {
		if entry.disconnectedAt != nil && now.Sub(*entry.disconnectedAt) > c.hostTTL {
			c.removeServiceHealthForHostLocked(hostID)
			delete(c.hosts, hostID)
		}
	}
	c.pruneServiceHealthLocked(now)
}

// ListManualServices returns user-managed manual services from persistence.
func (c *Coordinator) ListManualServices(hostAssetID string) ([]persistence.WebServiceManual, error) {
	if c.store == nil {
		return nil, ErrStoreUnavailable
	}
	return c.store.ListManualWebServices(strings.TrimSpace(hostAssetID))
}

// GetManualService returns one manual service by id.
func (c *Coordinator) GetManualService(id string) (persistence.WebServiceManual, bool, error) {
	if c.store == nil {
		return persistence.WebServiceManual{}, false, ErrStoreUnavailable
	}
	return c.store.GetManualWebService(strings.TrimSpace(id))
}

// SaveManualService creates or updates a manual service entry.
func (c *Coordinator) SaveManualService(service persistence.WebServiceManual) (persistence.WebServiceManual, error) {
	if c.store == nil {
		return persistence.WebServiceManual{}, ErrStoreUnavailable
	}
	saved, err := c.store.SaveManualWebService(service)
	if err == nil {
		c.invalidateStoreSnapshot()
	}
	return saved, err
}

// DeleteManualService removes a manual service entry.
func (c *Coordinator) DeleteManualService(id string) error {
	if c.store == nil {
		return ErrStoreUnavailable
	}
	err := c.store.DeleteManualWebService(strings.TrimSpace(id))
	if err == nil {
		c.invalidateStoreSnapshot()
	}
	return err
}

// ListOverrides returns configured web service overrides.
func (c *Coordinator) ListOverrides(hostAssetID string) ([]persistence.WebServiceOverride, error) {
	if c.store == nil {
		return nil, ErrStoreUnavailable
	}
	return c.store.ListWebServiceOverrides(strings.TrimSpace(hostAssetID))
}

// SaveOverride creates or updates one web service override.
func (c *Coordinator) SaveOverride(override persistence.WebServiceOverride) (persistence.WebServiceOverride, error) {
	if c.store == nil {
		return persistence.WebServiceOverride{}, ErrStoreUnavailable
	}
	saved, err := c.store.SaveWebServiceOverride(override)
	if err == nil {
		c.invalidateStoreSnapshot()
	}
	return saved, err
}

// DeleteOverride removes one web service override.
func (c *Coordinator) DeleteOverride(hostAssetID, serviceID string) error {
	if c.store == nil {
		return ErrStoreUnavailable
	}
	err := c.store.DeleteWebServiceOverride(strings.TrimSpace(hostAssetID), strings.TrimSpace(serviceID))
	if err == nil {
		c.invalidateStoreSnapshot()
	}
	return err
}

func (c *Coordinator) snapshotAllHostServices() []agentmgr.DiscoveredWebService {
	c.mu.RLock()
	defer c.mu.RUnlock()

	all := make([]agentmgr.DiscoveredWebService, 0, 64)
	for _, entry := range c.hosts {
		for _, svc := range entry.services {
			all = append(all, cloneDiscoveredService(svc))
		}
	}
	return all
}

func (c *Coordinator) snapshotHostServices(hostID string) []agentmgr.DiscoveredWebService {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.hosts[hostID]
	if !ok {
		return nil
	}
	out := make([]agentmgr.DiscoveredWebService, 0, len(entry.services))
	for _, svc := range entry.services {
		out = append(out, cloneDiscoveredService(svc))
	}
	return out
}

func (c *Coordinator) snapshotServiceSummaryEntries(
	hostFilter map[string]struct{},
) ([]serviceSummaryEntry, map[string]bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries := make([]serviceSummaryEntry, 0, 64)
	hostHasConsole := make(map[string]bool, len(c.hosts))
	for hostID, entry := range c.hosts {
		if entry == nil {
			continue
		}
		fallbackHostID := strings.TrimSpace(hostID)
		for _, svc := range entry.services {
			serviceHostID := strings.TrimSpace(svc.HostAssetID)
			if serviceHostID == "" {
				serviceHostID = fallbackHostID
			}
			if !summaryHostAllowed(serviceHostID, hostFilter) {
				continue
			}

			serviceKey := strings.TrimSpace(svc.ServiceKey)
			component := ""
			if serviceKey == labtetherServiceKey {
				component = classifyLabTetherComponent(svc)
				if component == labtetherConsole {
					hostHasConsole[serviceHostID] = true
				}
			}

			entries = append(entries, serviceSummaryEntry{
				hostAssetID: serviceHostID,
				serviceID:   strings.TrimSpace(svc.ID),
				status:      strings.TrimSpace(svc.Status),
				serviceKey:  serviceKey,
				component:   component,
				hidden:      isHiddenService(svc),
			})
		}
	}
	return entries, hostHasConsole
}

func summaryHostAllowed(hostID string, hostFilter map[string]struct{}) bool {
	if len(hostFilter) == 0 {
		return true
	}
	if hostID == "" {
		return true // standalone services are not host-scoped
	}
	_, ok := hostFilter[strings.TrimSpace(hostID)]
	return ok
}

func serviceHostMatchesFilter(hostAssetID, hostFilter string) bool {
	filter := strings.TrimSpace(hostFilter)
	if filter == "" {
		return true
	}
	return strings.TrimSpace(hostAssetID) == filter
}

func (c *Coordinator) mergeWithManualAndOverrides(base []agentmgr.DiscoveredWebService, hostFilter string) []agentmgr.DiscoveredWebService {
	if base == nil && c.store == nil {
		return nil
	}

	merged := make([]agentmgr.DiscoveredWebService, 0, len(base))
	merged = append(merged, base...)
	normalizeLabTetherPresentation(merged)

	if c.store == nil {
		return merged
	}

	snapshot, err := c.loadStoreSnapshot()
	if err != nil {
		log.Printf("webservice-coordinator: failed to load service store snapshot: %v", err)
		return merged
	}
	if snapshot == nil {
		return merged
	}
	manualCount := 0
	for _, manual := range snapshot.manuals {
		if serviceHostMatchesFilter(manual.HostAssetID, hostFilter) {
			manualCount++
		}
	}
	if cap(merged) < len(base)+manualCount {
		expanded := make([]agentmgr.DiscoveredWebService, 0, len(base)+manualCount)
		expanded = append(expanded, merged...)
		merged = expanded
	}

	seen := make(map[string]struct{}, len(merged))
	for _, svc := range merged {
		seen[serviceOverrideKey(svc.HostAssetID, svc.ID)] = struct{}{}
	}
	for _, manual := range snapshot.manuals {
		if !serviceHostMatchesFilter(manual.HostAssetID, hostFilter) {
			continue
		}
		asDiscovered := manualToDiscovered(manual)
		key := serviceOverrideKey(asDiscovered.HostAssetID, asDiscovered.ID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, asDiscovered)
	}

	for i := range merged {
		key := serviceOverrideKey(merged[i].HostAssetID, merged[i].ID)
		override, ok := snapshot.overrides[key]
		if !ok {
			continue
		}
		applyOverride(&merged[i], override)
	}

	return merged
}

func (c *Coordinator) loadStoreSnapshot() (*storeSnapshot, error) {
	if c == nil || c.store == nil {
		return nil, nil
	}

	c.storeCacheMu.RLock()
	cached := c.storeCache
	c.storeCacheMu.RUnlock()
	if cached != nil {
		return cached, nil
	}

	value, err, _ := c.storeLoad.Do("webservice-store-snapshot", func() (any, error) {
		c.storeCacheMu.RLock()
		loaded := c.storeCache
		c.storeCacheMu.RUnlock()
		if loaded != nil {
			return loaded, nil
		}

		manuals, err := c.store.ListManualWebServices("")
		if err != nil {
			return nil, err
		}

		overrides, err := c.store.ListWebServiceOverrides("")
		if err != nil {
			return nil, err
		}

		overrideMap := make(map[string]persistence.WebServiceOverride, len(overrides))
		for _, override := range overrides {
			overrideMap[serviceOverrideKey(override.HostAssetID, override.ServiceID)] = override
		}

		snapshot := &storeSnapshot{
			manuals:   append([]persistence.WebServiceManual(nil), manuals...),
			overrides: overrideMap,
		}

		c.storeCacheMu.Lock()
		c.storeCache = snapshot
		c.storeCacheMu.Unlock()
		return snapshot, nil
	})
	if err != nil {
		return nil, err
	}

	snapshot, _ := value.(*storeSnapshot)
	return snapshot, nil
}

func (c *Coordinator) invalidateStoreSnapshot() {
	if c == nil {
		return
	}
	c.storeCacheMu.Lock()
	c.storeCache = nil
	c.storeCacheMu.Unlock()
}

func normalizeLabTetherPresentation(services []agentmgr.DiscoveredWebService) {
	if len(services) == 0 {
		return
	}

	hostHasConsole := make(map[string]bool)

	for i := range services {
		svc := &services[i]
		if strings.TrimSpace(svc.ServiceKey) != labtetherServiceKey {
			continue
		}

		component := classifyLabTetherComponent(*svc)
		if component == "" {
			continue
		}

		if svc.Metadata == nil {
			svc.Metadata = make(map[string]string)
		}
		svc.Metadata["labtether_component"] = component

		switch component {
		case labtetherConsole:
			svc.Name = "LabTether Console"
			hostHasConsole[strings.TrimSpace(svc.HostAssetID)] = true
			delete(svc.Metadata, "hidden")
		case labtetherAPI:
			svc.Name = "LabTether API"
		}
	}

	for i := range services {
		svc := &services[i]
		if strings.TrimSpace(svc.ServiceKey) != labtetherServiceKey || svc.Metadata == nil {
			continue
		}
		if svc.Metadata["labtether_component"] != labtetherAPI {
			continue
		}
		if hostHasConsole[strings.TrimSpace(svc.HostAssetID)] {
			svc.Metadata["hidden"] = "true"
		} else {
			delete(svc.Metadata, "hidden")
		}
	}
}

func classifyLabTetherComponent(svc agentmgr.DiscoveredWebService) string {
	if strings.TrimSpace(svc.ServiceKey) != labtetherServiceKey {
		return ""
	}

	if svc.Metadata != nil {
		switch strings.ToLower(strings.TrimSpace(svc.Metadata["labtether_component"])) {
		case labtetherConsole:
			return labtetherConsole
		case labtetherAPI:
			return labtetherAPI
		}
	}

	port := servicePortFromURL(svc.URL)
	if port == 0 && svc.Metadata != nil {
		port = servicePortFromURL(svc.Metadata["raw_url"])
	}
	if port == 0 && svc.Metadata != nil {
		port = servicePortFromURL(svc.Metadata["backend_url"])
	}

	switch port {
	case 3000:
		return labtetherConsole
	case 8080, 8443:
		return labtetherAPI
	}

	if svc.Metadata != nil {
		switch strings.TrimSpace(svc.Metadata["health_path"]) {
		case "/api/health":
			return labtetherConsole
		case "/healthz", "/version":
			return labtetherAPI
		}
	}

	return ""
}

func servicePortFromURL(raw string) int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return 0
	}
	port := strings.TrimSpace(parsed.Port())
	if port == "" {
		return 0
	}
	value, err := strconv.Atoi(port)
	if err != nil || value <= 0 || value > 65535 {
		return 0
	}
	return value
}

func applyOverride(svc *agentmgr.DiscoveredWebService, override persistence.WebServiceOverride) {
	if svc == nil {
		return
	}
	if strings.TrimSpace(override.NameOverride) != "" {
		svc.Name = strings.TrimSpace(override.NameOverride)
	}
	if strings.TrimSpace(override.CategoryOverride) != "" {
		svc.Category = strings.TrimSpace(override.CategoryOverride)
	}
	if strings.TrimSpace(override.URLOverride) != "" {
		svc.URL = strings.TrimSpace(override.URLOverride)
	}
	if strings.TrimSpace(override.IconKeyOverride) != "" {
		svc.IconKey = strings.TrimSpace(override.IconKeyOverride)
	}
	if svc.Metadata == nil {
		svc.Metadata = make(map[string]string)
	}
	if tags := strings.TrimSpace(override.TagsOverride); tags != "" {
		svc.Metadata["user_tags"] = tags
	} else {
		delete(svc.Metadata, "user_tags")
	}
	if override.Hidden {
		svc.Metadata["hidden"] = "true"
	} else {
		delete(svc.Metadata, "hidden")
	}
}

func manualToDiscovered(manual persistence.WebServiceManual) agentmgr.DiscoveredWebService {
	metadata := cloneMetadata(manual.Metadata)
	if metadata == nil {
		metadata = make(map[string]string)
	}
	metadata["manual"] = "true"

	return agentmgr.DiscoveredWebService{
		ID:          manual.ID,
		Name:        manual.Name,
		Category:    manual.Category,
		URL:         manual.URL,
		Source:      "manual",
		Status:      "unknown",
		HostAssetID: manual.HostAssetID,
		IconKey:     manual.IconKey,
		Metadata:    metadata,
	}
}

func serviceOverrideKey(hostID, serviceID string) string {
	return strings.TrimSpace(hostID) + "::" + strings.TrimSpace(serviceID)
}

func cloneDiscoveredService(in agentmgr.DiscoveredWebService) agentmgr.DiscoveredWebService {
	out := in
	out.Metadata = cloneMetadata(in.Metadata)
	out.Health = cloneWebServiceHealthSummary(in.Health)
	return out
}

func cloneWebServiceDiscoveryStats(in *agentmgr.WebServiceDiscoveryStats) *agentmgr.WebServiceDiscoveryStats {
	if in == nil {
		return nil
	}
	out := *in
	if len(in.Sources) > 0 {
		out.Sources = make(map[string]agentmgr.WebServiceDiscoverySourceStat, len(in.Sources))
		for key, value := range in.Sources {
			out.Sources[key] = value
		}
	}
	if len(in.FinalSourceCount) > 0 {
		out.FinalSourceCount = make(map[string]int, len(in.FinalSourceCount))
		for key, value := range in.FinalSourceCount {
			out.FinalSourceCount[key] = value
		}
	}
	return &out
}

func cloneMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneWebServiceHealthSummary(in *agentmgr.WebServiceHealthSummary) *agentmgr.WebServiceHealthSummary {
	if in == nil {
		return nil
	}
	out := *in
	if len(in.Recent) > 0 {
		out.Recent = append([]agentmgr.WebServiceHealthPoint(nil), in.Recent...)
	}
	return &out
}

func (c *Coordinator) now() time.Time {
	if c == nil || c.nowFn == nil {
		return time.Now()
	}
	return c.nowFn()
}

func normalizeServiceHostID(candidates ...string) string {
	for _, candidate := range candidates {
		trimmed := strings.TrimSpace(candidate)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeServiceStatus(status string) string {
	trimmed := strings.ToLower(strings.TrimSpace(status))
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}

func trimServiceHealthSamples(
	samples []serviceHealthSample,
	cutoff time.Time,
	maxSamples int,
) []serviceHealthSample {
	if len(samples) == 0 {
		return nil
	}
	start := 0
	for start < len(samples) && samples[start].at.Before(cutoff) {
		start++
	}
	if start >= len(samples) {
		return nil
	}
	trimmed := samples[start:]
	if maxSamples > 0 && len(trimmed) > maxSamples {
		trimmed = trimmed[len(trimmed)-maxSamples:]
	}
	return trimmed
}

func buildServiceHealthSummary(
	history *serviceHealthHistory,
	now time.Time,
) *agentmgr.WebServiceHealthSummary {
	if history == nil || len(history.samples) == 0 {
		return nil
	}
	cutoff := now.Add(-serviceHealthWindow)
	samples := trimServiceHealthSamples(history.samples, cutoff, 0)
	if len(samples) == 0 {
		return nil
	}

	upChecks := 0
	lastChangeAt := time.Time{}
	previousStatus := normalizeServiceStatus(samples[0].status)
	for index, sample := range samples {
		normalizedStatus := normalizeServiceStatus(sample.status)
		if strings.EqualFold(normalizedStatus, "up") {
			upChecks++
		}
		if index > 0 && normalizedStatus != previousStatus {
			lastChangeAt = sample.at
		}
		previousStatus = normalizedStatus
	}

	checks := len(samples)
	uptimePercent := 0.0
	if checks > 0 {
		uptimePercent = (float64(upChecks) / float64(checks)) * 100
		uptimePercent = math.Round(uptimePercent*10) / 10
	}

	recentStart := 0
	if len(samples) > serviceHealthRecentLimit {
		recentStart = len(samples) - serviceHealthRecentLimit
	}
	recent := make([]agentmgr.WebServiceHealthPoint, 0, len(samples)-recentStart)
	for _, sample := range samples[recentStart:] {
		recent = append(recent, agentmgr.WebServiceHealthPoint{
			At:         sample.at.UTC().Format(time.RFC3339),
			Status:     normalizeServiceStatus(sample.status),
			ResponseMs: sample.responseMs,
		})
	}

	summary := &agentmgr.WebServiceHealthSummary{
		Window:        serviceHealthWindowLabel,
		Checks:        checks,
		UpChecks:      upChecks,
		UptimePercent: uptimePercent,
		LastCheckedAt: samples[len(samples)-1].at.UTC().Format(time.RFC3339),
		Recent:        recent,
	}
	if !lastChangeAt.IsZero() {
		summary.LastChangeAt = lastChangeAt.UTC().Format(time.RFC3339)
	}
	return summary
}

func (c *Coordinator) recordServiceHealthSampleLocked(
	hostID string,
	serviceID string,
	status string,
	responseMs int,
	at time.Time,
) {
	if c == nil {
		return
	}
	hostID = strings.TrimSpace(hostID)
	serviceID = strings.TrimSpace(serviceID)
	if hostID == "" || serviceID == "" {
		return
	}
	key := serviceOverrideKey(hostID, serviceID)
	history, ok := c.serviceHealth[key]
	if !ok || history == nil {
		history = &serviceHealthHistory{}
		c.serviceHealth[key] = history
	}

	normalizedStatus := normalizeServiceStatus(status)
	if count := len(history.samples); count > 0 {
		last := &history.samples[count-1]
		if normalizeServiceStatus(last.status) == normalizedStatus && at.Sub(last.at) <= serviceHealthCoalesce {
			last.at = at
			last.status = normalizedStatus
			last.responseMs = responseMs
			return
		}
	}

	history.samples = append(history.samples, serviceHealthSample{
		at:         at,
		status:     normalizedStatus,
		responseMs: responseMs,
	})
	history.samples = trimServiceHealthSamples(history.samples, at.Add(-serviceHealthWindow), serviceHealthMaxSamples)
	if len(history.samples) == 0 {
		delete(c.serviceHealth, key)
	}
}

func (c *Coordinator) removeServiceHealthForHostLocked(hostID string) {
	if c == nil {
		return
	}
	trimmedHostID := strings.TrimSpace(hostID)
	if trimmedHostID == "" {
		return
	}
	prefix := trimmedHostID + "::"
	for key := range c.serviceHealth {
		if strings.HasPrefix(key, prefix) {
			delete(c.serviceHealth, key)
		}
	}
}

func (c *Coordinator) pruneServiceHealthLocked(now time.Time) {
	if c == nil {
		return
	}
	cutoff := now.Add(-serviceHealthWindow)
	for key, history := range c.serviceHealth {
		if history == nil {
			delete(c.serviceHealth, key)
			continue
		}
		history.samples = trimServiceHealthSamples(history.samples, cutoff, serviceHealthMaxSamples)
		if len(history.samples) == 0 {
			delete(c.serviceHealth, key)
		}
	}
}

func isHiddenService(svc agentmgr.DiscoveredWebService) bool {
	if svc.Metadata == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(svc.Metadata["hidden"]), "true")
}

// enrichServicesFromRegistry applies the hub's service registry to fill in
// missing category, icon, and name data. This ensures services are correctly
// classified even when reported by agents running older registry versions.
func enrichServicesFromRegistry(services []agentmgr.DiscoveredWebService) {
	for i := range services {
		svc := &services[i]
		// Skip services already classified (have a service key and icon).
		if svc.ServiceKey != "" && svc.IconKey != "" {
			continue
		}
		// Try matching by Docker image first.
		if image := svc.Metadata["image"]; image != "" {
			if known, ok := agentcore.LookupByDockerImage(image); ok {
				applyRegistryMatch(svc, known)
				continue
			}
		}
		// Try matching by name, URL domain, or router name hints.
		hints := []string{svc.Name}
		if svc.Metadata != nil {
			if rn := svc.Metadata["router_name"]; rn != "" {
				hints = append(hints, rn)
			}
		}
		if svc.URL != "" {
			hints = append(hints, svc.URL)
		}
		for _, hint := range hints {
			if known, ok := agentcore.LookupByHint(hint); ok {
				applyRegistryMatch(svc, known)
				break
			}
		}
	}
}

// applyRegistryMatch fills in missing fields from a registry match without
// overwriting values the agent already set.
func applyRegistryMatch(svc *agentmgr.DiscoveredWebService, known agentcore.KnownService) {
	if svc.ServiceKey == "" {
		svc.ServiceKey = known.Key
	}
	if svc.IconKey == "" {
		svc.IconKey = known.IconKey
	}
	if svc.Category == "" || svc.Category == agentcore.CatOther {
		svc.Category = known.Category
	}
	// Only update name if it looks auto-generated (e.g. "Port 8080" or container name).
	if svc.Name == "" {
		svc.Name = known.Name
	}
}
