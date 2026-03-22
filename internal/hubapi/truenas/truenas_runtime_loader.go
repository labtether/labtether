package truenas

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	tnconnector "github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubcollector"
)

func (d *Deps) LoadTrueNASRuntime(collectorID string) (*TruenasRuntime, error) {
	if d.HubCollectorStore == nil {
		return nil, fmt.Errorf("hub collector store unavailable")
	}
	if d.CredentialStore == nil || d.SecretsManager == nil {
		return nil, fmt.Errorf("credential store unavailable")
	}

	collectors, err := d.HubCollectorStore.ListHubCollectors(200, true)
	if err != nil {
		return nil, fmt.Errorf("failed to list hub collectors: %w", err)
	}

	selected := selectCollectorForTrueNASRuntime(collectors, collectorID)
	if selected == nil {
		return nil, fmt.Errorf("no active truenas collector configured")
	}

	baseURL := shared.CollectorConfigString(selected.Config, "base_url")
	credentialID := shared.CollectorConfigString(selected.Config, "credential_id")
	if baseURL == "" || credentialID == "" {
		return nil, fmt.Errorf("truenas collector config is incomplete")
	}

	credential, ok, err := d.CredentialStore.GetCredentialProfile(credentialID)
	if err != nil || !ok {
		return nil, fmt.Errorf("truenas credential profile not found")
	}

	decryptedSecret, err := d.SecretsManager.DecryptString(credential.SecretCiphertext, credential.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt truenas credential")
	}

	skipVerify, hasSkip := shared.CollectorConfigBool(selected.Config, "skip_verify")
	if !hasSkip {
		skipVerify = false
	}
	timeout := shared.CollectorConfigDuration(selected.Config, "timeout", 15*time.Second)

	secretHash := fmt.Sprintf("%x", sha256.Sum256([]byte(decryptedSecret)))
	configKey := fmt.Sprintf("%s|%s|%v|%s|%s", strings.TrimSpace(baseURL), strings.TrimSpace(credentialID), skipVerify, timeout.String(), secretHash)

	// Cache lookup under read lock.
	d.TruenasCacheMu.RLock()
	if cached, hit := d.TruenasCache[selected.ID]; hit && cached.ConfigKey == configKey {
		d.TruenasCacheMu.RUnlock()
		return cached.Runtime, nil
	}
	d.TruenasCacheMu.RUnlock()

	runtime := &TruenasRuntime{
		Client: &tnconnector.Client{
			BaseURL:    strings.TrimSpace(baseURL),
			APIKey:     strings.TrimSpace(decryptedSecret),
			SkipVerify: skipVerify,
			Timeout:    timeout,
		},
		CollectorID: strings.TrimSpace(selected.ID),
		BaseURL:     strings.TrimSpace(baseURL),
		APIKey:      strings.TrimSpace(decryptedSecret),
		SkipVerify:  skipVerify,
		Timeout:     timeout,
		ConfigKey:   configKey,
	}

	// Cache update under write lock.
	d.TruenasCacheMu.Lock()
	if d.TruenasCache == nil {
		d.TruenasCache = make(map[string]*CachedTrueNASRuntime)
	}
	if len(d.TruenasCache) >= shared.MaxRuntimeCacheEntries {
		d.TruenasCache = make(map[string]*CachedTrueNASRuntime)
	}
	d.TruenasCache[selected.ID] = &CachedTrueNASRuntime{
		Runtime:   runtime,
		ConfigKey: configKey,
	}
	d.TruenasCacheMu.Unlock()

	return runtime, nil
}

func selectCollectorForTrueNASRuntime(collectors []hubcollector.Collector, collectorID string) *hubcollector.Collector {
	collectorID = strings.TrimSpace(collectorID)

	if collectorID != "" {
		for i := range collectors {
			current := collectors[i]
			if current.CollectorType != hubcollector.CollectorTypeTrueNAS {
				continue
			}
			if current.ID == collectorID {
				return &current
			}
		}
		return nil
	}

	for i := range collectors {
		current := collectors[i]
		if current.CollectorType != hubcollector.CollectorTypeTrueNAS {
			continue
		}
		return &current
	}

	return nil
}
