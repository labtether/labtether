package webservice

import (
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/persistence"
	"log"
	"strings"
)

type storeSnapshot struct {
	manuals   []persistence.WebServiceManual
	overrides map[string]persistence.WebServiceOverride
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
