package persistence

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/idgen"
)

type MemoryRuntimeSettingsStore struct {
	mu     sync.RWMutex
	values map[string]string
}

func NewMemoryRuntimeSettingsStore() *MemoryRuntimeSettingsStore {
	return &MemoryRuntimeSettingsStore{
		values: make(map[string]string),
	}
}

func (m *MemoryRuntimeSettingsStore) ListRuntimeSettingOverrides() (map[string]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneRuntimeSettingsMap(m.values), nil
}

func (m *MemoryRuntimeSettingsStore) SaveRuntimeSettingOverrides(values map[string]string) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, value := range values {
		m.values[key] = value
	}
	return cloneRuntimeSettingsMap(m.values), nil
}

func (m *MemoryRuntimeSettingsStore) DeleteRuntimeSettingOverrides(keys []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(keys) == 0 {
		m.values = make(map[string]string)
		return nil
	}
	for _, key := range keys {
		delete(m.values, key)
	}
	return nil
}

func cloneRuntimeSettingsMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

type MemoryCredentialStore struct {
	mu             sync.RWMutex
	profiles       map[string]credentials.Profile
	terminalConfig map[string]credentials.AssetTerminalConfig
	desktopConfig  map[string]credentials.AssetDesktopConfig
}

func NewMemoryCredentialStore() *MemoryCredentialStore {
	return &MemoryCredentialStore{
		profiles:       make(map[string]credentials.Profile),
		terminalConfig: make(map[string]credentials.AssetTerminalConfig),
		desktopConfig:  make(map[string]credentials.AssetDesktopConfig),
	}
}

func (m *MemoryCredentialStore) CreateCredentialProfile(profile credentials.Profile) (credentials.Profile, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(profile.ID) == "" {
		profile.ID = idgen.New("cred")
	}
	profile.Name = strings.TrimSpace(profile.Name)
	profile.Kind = strings.TrimSpace(profile.Kind)
	profile.Username = strings.TrimSpace(profile.Username)
	profile.Description = strings.TrimSpace(profile.Description)
	profile.Status = strings.TrimSpace(profile.Status)
	if profile.Status == "" {
		profile.Status = "active"
	}
	profile.Metadata = cloneMetadata(profile.Metadata)
	profile.CreatedAt = now
	profile.UpdatedAt = now
	if profile.RotatedAt == nil {
		profile.RotatedAt = &now
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.profiles[profile.ID] = cloneCredentialProfile(profile)
	return cloneCredentialProfile(profile), nil
}

func (m *MemoryCredentialStore) UpdateCredentialProfile(profile credentials.Profile) (credentials.Profile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.profiles[strings.TrimSpace(profile.ID)]
	if !ok {
		return credentials.Profile{}, errors.New("credential profile not found")
	}

	existing.Name = strings.TrimSpace(profile.Name)
	existing.Kind = strings.TrimSpace(profile.Kind)
	existing.Username = strings.TrimSpace(profile.Username)
	existing.Description = strings.TrimSpace(profile.Description)
	existing.Status = strings.TrimSpace(profile.Status)
	if existing.Status == "" {
		existing.Status = "active"
	}
	existing.Metadata = cloneMetadata(profile.Metadata)
	existing.SecretCiphertext = strings.TrimSpace(profile.SecretCiphertext)
	existing.PassphraseCiphertext = strings.TrimSpace(profile.PassphraseCiphertext)
	existing.UpdatedAt = time.Now().UTC()
	existing.RotatedAt = cloneTimePtr(profile.RotatedAt)
	existing.ExpiresAt = cloneTimePtr(profile.ExpiresAt)

	m.profiles[existing.ID] = cloneCredentialProfile(existing)
	return cloneCredentialProfile(existing), nil
}

func (m *MemoryCredentialStore) UpdateCredentialProfileSecret(id, secretCiphertext, passphraseCiphertext string, expiresAt *time.Time) (credentials.Profile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	profile, ok := m.profiles[id]
	if !ok {
		return credentials.Profile{}, errors.New("credential profile not found")
	}

	now := time.Now().UTC()
	profile.SecretCiphertext = strings.TrimSpace(secretCiphertext)
	profile.PassphraseCiphertext = strings.TrimSpace(passphraseCiphertext)
	profile.UpdatedAt = now
	profile.RotatedAt = &now
	if expiresAt != nil {
		exp := expiresAt.UTC()
		profile.ExpiresAt = &exp
	} else {
		profile.ExpiresAt = nil
	}

	m.profiles[id] = cloneCredentialProfile(profile)
	return cloneCredentialProfile(profile), nil
}

func (m *MemoryCredentialStore) GetCredentialProfile(id string) (credentials.Profile, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	profile, ok := m.profiles[id]
	if !ok {
		return credentials.Profile{}, false, nil
	}
	return cloneCredentialProfile(profile), true, nil
}

func (m *MemoryCredentialStore) ListCredentialProfiles(limit int) ([]credentials.Profile, error) {
	if limit <= 0 {
		limit = 50
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]credentials.Profile, 0, len(m.profiles))
	for _, profile := range m.profiles {
		out = append(out, cloneCredentialProfile(profile))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryCredentialStore) MarkCredentialProfileUsed(id string, usedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	profile, ok := m.profiles[id]
	if !ok {
		return errors.New("credential profile not found")
	}
	t := usedAt.UTC()
	profile.LastUsedAt = &t
	profile.UpdatedAt = time.Now().UTC()
	m.profiles[id] = cloneCredentialProfile(profile)
	return nil
}

func (m *MemoryCredentialStore) DeleteCredentialProfile(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.profiles[id]; !ok {
		return ErrNotFound
	}
	delete(m.profiles, id)
	return nil
}

func (m *MemoryCredentialStore) SaveAssetTerminalConfig(cfg credentials.AssetTerminalConfig) (credentials.AssetTerminalConfig, error) {
	cfg.AssetID = strings.TrimSpace(cfg.AssetID)
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.HostKey = strings.TrimSpace(cfg.HostKey)
	cfg.CredentialProfileID = strings.TrimSpace(cfg.CredentialProfileID)
	if cfg.Port <= 0 {
		cfg.Port = 22
	}
	cfg.UpdatedAt = time.Now().UTC()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.terminalConfig[cfg.AssetID] = cfg
	return cfg, nil
}

func (m *MemoryCredentialStore) GetAssetTerminalConfig(assetID string) (credentials.AssetTerminalConfig, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg, ok := m.terminalConfig[strings.TrimSpace(assetID)]
	if !ok {
		return credentials.AssetTerminalConfig{}, false, nil
	}
	return cfg, true, nil
}

func (m *MemoryCredentialStore) DeleteAssetTerminalConfig(assetID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := strings.TrimSpace(assetID)
	if _, ok := m.terminalConfig[id]; !ok {
		return ErrNotFound
	}
	delete(m.terminalConfig, id)
	return nil
}

func (m *MemoryCredentialStore) SaveDesktopConfig(cfg credentials.AssetDesktopConfig) (credentials.AssetDesktopConfig, error) {
	cfg.AssetID = strings.TrimSpace(cfg.AssetID)
	if cfg.VNCPort <= 0 {
		cfg.VNCPort = 5900
	}
	cfg.UpdatedAt = time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.desktopConfig[cfg.AssetID] = cfg
	return cfg, nil
}

func (m *MemoryCredentialStore) GetDesktopConfig(assetID string) (credentials.AssetDesktopConfig, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg, ok := m.desktopConfig[strings.TrimSpace(assetID)]
	if !ok {
		return credentials.AssetDesktopConfig{}, false, nil
	}
	return cfg, true, nil
}

func (m *MemoryCredentialStore) DeleteDesktopConfig(assetID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := strings.TrimSpace(assetID)
	if _, ok := m.desktopConfig[id]; !ok {
		return ErrNotFound
	}
	delete(m.desktopConfig, id)
	return nil
}

func cloneCredentialProfile(input credentials.Profile) credentials.Profile {
	out := input
	out.Metadata = cloneMetadata(input.Metadata)
	if input.RotatedAt != nil {
		value := input.RotatedAt.UTC()
		out.RotatedAt = &value
	}
	if input.LastUsedAt != nil {
		value := input.LastUsedAt.UTC()
		out.LastUsedAt = &value
	}
	if input.ExpiresAt != nil {
		value := input.ExpiresAt.UTC()
		out.ExpiresAt = &value
	}
	return out
}
