package webservice

import (
	"encoding/json"
	"errors"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/persistence"
	"golang.org/x/sync/singleflight"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
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
		services = append(services, svc)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for _, svc := range services {
		c.recordServiceHealthSampleLocked(svc.HostAssetID, svc.ID, svc.Status, svc.ResponseMs, now)
	}

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
