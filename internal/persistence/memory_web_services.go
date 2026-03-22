package persistence

import (
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/idgen"
)

type MemoryWebServiceStore struct {
	mu        sync.RWMutex
	manuals   map[string]WebServiceManual
	overrides map[string]WebServiceOverride
}

func NewMemoryWebServiceStore() *MemoryWebServiceStore {
	return &MemoryWebServiceStore{
		manuals:   make(map[string]WebServiceManual),
		overrides: make(map[string]WebServiceOverride),
	}
}

func (m *MemoryWebServiceStore) ListManualWebServices(hostAssetID string) ([]WebServiceManual, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []WebServiceManual
	for _, svc := range m.manuals {
		if hostAssetID == "" || svc.HostAssetID == hostAssetID {
			result = append(result, cloneManualWebService(svc))
		}
	}
	return result, nil
}

func (m *MemoryWebServiceStore) PromoteManualServicesToStandalone(hostAssetID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, svc := range m.manuals {
		if svc.HostAssetID == hostAssetID {
			svc.HostAssetID = ""
			svc.UpdatedAt = time.Now()
			m.manuals[id] = svc
		}
	}
	return nil
}

func (m *MemoryWebServiceStore) GetManualWebService(id string) (WebServiceManual, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	svc, ok := m.manuals[strings.TrimSpace(id)]
	if !ok {
		return WebServiceManual{}, false, nil
	}
	return cloneManualWebService(svc), true, nil
}

func (m *MemoryWebServiceStore) SaveManualWebService(service WebServiceManual) (WebServiceManual, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	id := strings.TrimSpace(service.ID)
	if id == "" {
		id = idgen.New("wsvc")
	}

	existing, hasExisting := m.manuals[id]
	if !hasExisting {
		existing.CreatedAt = now
	}

	existing.ID = id
	existing.HostAssetID = strings.TrimSpace(service.HostAssetID)
	existing.Name = strings.TrimSpace(service.Name)
	existing.Category = strings.TrimSpace(service.Category)
	existing.URL = strings.TrimSpace(service.URL)
	existing.IconKey = strings.TrimSpace(service.IconKey)
	existing.Metadata = cloneMetadata(service.Metadata)
	existing.UpdatedAt = now

	m.manuals[id] = existing
	return cloneManualWebService(existing), nil
}

func (m *MemoryWebServiceStore) DeleteManualWebService(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := strings.TrimSpace(id)
	if _, ok := m.manuals[key]; !ok {
		return ErrNotFound
	}
	delete(m.manuals, key)
	return nil
}

func (m *MemoryWebServiceStore) ListWebServiceOverrides(hostAssetID string) ([]WebServiceOverride, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	host := strings.TrimSpace(hostAssetID)
	out := make([]WebServiceOverride, 0, len(m.overrides))
	for _, override := range m.overrides {
		if host != "" && override.HostAssetID != host {
			continue
		}
		out = append(out, cloneWebServiceOverride(override))
	}
	return out, nil
}

func (m *MemoryWebServiceStore) SaveWebServiceOverride(override WebServiceOverride) (WebServiceOverride, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	normalized := WebServiceOverride{
		HostAssetID:      strings.TrimSpace(override.HostAssetID),
		ServiceID:        strings.TrimSpace(override.ServiceID),
		NameOverride:     strings.TrimSpace(override.NameOverride),
		CategoryOverride: strings.TrimSpace(override.CategoryOverride),
		URLOverride:      strings.TrimSpace(override.URLOverride),
		IconKeyOverride:  strings.TrimSpace(override.IconKeyOverride),
		TagsOverride:     strings.TrimSpace(override.TagsOverride),
		Hidden:           override.Hidden,
		UpdatedAt:        time.Now().UTC(),
	}

	m.overrides[overrideKey(normalized.HostAssetID, normalized.ServiceID)] = normalized
	return cloneWebServiceOverride(normalized), nil
}

func (m *MemoryWebServiceStore) DeleteWebServiceOverride(hostAssetID, serviceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := overrideKey(hostAssetID, serviceID)
	if _, ok := m.overrides[key]; !ok {
		return ErrNotFound
	}
	delete(m.overrides, key)
	return nil
}

func overrideKey(hostAssetID, serviceID string) string {
	return strings.TrimSpace(hostAssetID) + "::" + strings.TrimSpace(serviceID)
}

func cloneManualWebService(in WebServiceManual) WebServiceManual {
	out := in
	out.Metadata = cloneMetadata(in.Metadata)
	return out
}

func cloneWebServiceOverride(in WebServiceOverride) WebServiceOverride {
	return in
}
