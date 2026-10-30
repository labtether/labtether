package persistence

import (
	"errors"
	"strings"
	"sync"
	"time"
)

// HubSSHKeySetting is the per-device "Hub SSH key" choice, kept on the Hub.
// A device with no stored setting has the key off: devices added after the
// setting existed start off, and the upgrade migration stored "on" for every
// device that existed before it.
type HubSSHKeySetting struct {
	AssetID string `json:"asset_id"`
	Enabled bool   `json:"enabled"`
	// RemovalPending marks a device that had the key and was turned off while
	// the Hub could not reach its agent. The Hub asks the agent to remove the
	// key when it next connects.
	RemovalPending bool      `json:"removal_pending"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
}

// HubSSHKeyStore keeps the per-device Hub SSH key setting.
type HubSSHKeyStore interface {
	// GetHubSSHKeySetting returns the stored setting, or an "off" setting when
	// none is stored.
	GetHubSSHKeySetting(assetID string) (HubSSHKeySetting, error)
	// SetHubSSHKeySetting stores the setting for an existing device. A pending
	// removal is only kept while the key is off.
	SetHubSSHKeySetting(assetID string, enabled, removalPending bool) (HubSSHKeySetting, error)
	// ClearHubSSHKeyRemovalPending clears a pending removal, but only while the
	// key is still off.
	ClearHubSSHKeyRemovalPending(assetID string) error
}

var errHubSSHKeyAssetIDRequired = errors.New("asset id is required")

// MemoryHubSSHKeyStore is the in-memory HubSSHKeyStore used by tests and
// database-free runs.
type MemoryHubSSHKeyStore struct {
	mu       sync.Mutex
	settings map[string]HubSSHKeySetting
}

func NewMemoryHubSSHKeyStore() *MemoryHubSSHKeyStore {
	return &MemoryHubSSHKeyStore{settings: make(map[string]HubSSHKeySetting)}
}

func (m *MemoryHubSSHKeyStore) GetHubSSHKeySetting(assetID string) (HubSSHKeySetting, error) {
	id := strings.TrimSpace(assetID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if setting, ok := m.settings[id]; ok {
		return setting, nil
	}
	return HubSSHKeySetting{AssetID: id}, nil
}

func (m *MemoryHubSSHKeyStore) SetHubSSHKeySetting(assetID string, enabled, removalPending bool) (HubSSHKeySetting, error) {
	id := strings.TrimSpace(assetID)
	if id == "" {
		return HubSSHKeySetting{}, errHubSSHKeyAssetIDRequired
	}
	setting := HubSSHKeySetting{
		AssetID:        id,
		Enabled:        enabled,
		RemovalPending: removalPending && !enabled,
		UpdatedAt:      time.Now().UTC(),
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings[id] = setting
	return setting, nil
}

func (m *MemoryHubSSHKeyStore) ClearHubSSHKeyRemovalPending(assetID string) error {
	id := strings.TrimSpace(assetID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if setting, ok := m.settings[id]; ok && !setting.Enabled {
		setting.RemovalPending = false
		setting.UpdatedAt = time.Now().UTC()
		m.settings[id] = setting
	}
	return nil
}
