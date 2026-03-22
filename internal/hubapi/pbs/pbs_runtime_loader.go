package pbs

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	pbsconnector "github.com/labtether/labtether/internal/connectors/pbs"
	"github.com/labtether/labtether/internal/hubcollector"
)

func (d *Deps) LoadPBSRuntime(collectorID string) (*PBSRuntime, error) {
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
	if strings.TrimSpace(collectorID) == "" && hasMultipleActivePBSCollectors(collectors) {
		return nil, fmt.Errorf("collector_id is required when multiple active pbs collectors are configured")
	}

	selected := SelectCollectorForPBSRuntime(collectors, collectorID)
	if selected == nil {
		return nil, fmt.Errorf("no active pbs collector configured")
	}

	baseURL := shared.CollectorConfigString(selected.Config, "base_url")
	credentialID := shared.CollectorConfigString(selected.Config, "credential_id")
	if baseURL == "" || credentialID == "" {
		return nil, fmt.Errorf("pbs collector config is incomplete")
	}

	credential, ok, err := d.CredentialStore.GetCredentialProfile(credentialID)
	if err != nil || !ok {
		return nil, fmt.Errorf("pbs credential profile not found")
	}

	decryptedSecret, err := d.SecretsManager.DecryptString(credential.SecretCiphertext, credential.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt pbs credential")
	}

	tokenID := shared.CollectorConfigString(selected.Config, "token_id")
	if tokenID == "" {
		tokenID = strings.TrimSpace(credential.Username)
	}
	if tokenID == "" {
		tokenID = strings.TrimSpace(credential.Metadata["token_id"])
	}
	if tokenID == "" {
		return nil, fmt.Errorf("pbs token_id missing in collector config")
	}

	skipVerify, hasSkip := shared.CollectorConfigBool(selected.Config, "skip_verify")
	if !hasSkip {
		skipVerify = false
	}
	timeout := shared.CollectorConfigDuration(selected.Config, "timeout", 15*time.Second)
	caPEM := shared.CollectorConfigString(selected.Config, "ca_pem")

	secretHash := fmt.Sprintf("%x", sha256.Sum256([]byte(decryptedSecret)))
	configKey := fmt.Sprintf("%s|%s|%s|%v|%s|%s|%s",
		strings.TrimSpace(baseURL),
		strings.TrimSpace(credentialID),
		strings.TrimSpace(tokenID),
		skipVerify,
		strings.TrimSpace(caPEM),
		timeout.String(),
		secretHash,
	)

	d.PBSCacheMu.RLock()
	if cached, hit := d.PBSCache[selected.ID]; hit && cached.ConfigKey == configKey {
		d.PBSCacheMu.RUnlock()
		return cached.Runtime, nil
	}
	d.PBSCacheMu.RUnlock()

	client, err := pbsconnector.NewClient(pbsconnector.Config{
		BaseURL:     strings.TrimSpace(baseURL),
		TokenID:     strings.TrimSpace(tokenID),
		TokenSecret: strings.TrimSpace(decryptedSecret),
		SkipVerify:  skipVerify,
		CAPEM:       caPEM,
		Timeout:     timeout,
	})
	if err != nil {
		return nil, err
	}

	runtime := &PBSRuntime{
		Client:      client,
		CollectorID: strings.TrimSpace(selected.ID),
		BaseURL:     strings.TrimSpace(baseURL),
		TokenID:     strings.TrimSpace(tokenID),
		SkipVerify:  skipVerify,
		CaPEM:       strings.TrimSpace(caPEM),
		Timeout:     timeout,
		ConfigKey:   configKey,
	}

	d.PBSCacheMu.Lock()
	if d.PBSCache == nil {
		d.PBSCache = make(map[string]*CachedPBSRuntime)
	}
	if len(d.PBSCache) >= shared.MaxRuntimeCacheEntries {
		d.PBSCache = make(map[string]*CachedPBSRuntime)
	}
	d.PBSCache[selected.ID] = &CachedPBSRuntime{
		Runtime:   runtime,
		ConfigKey: configKey,
	}
	d.PBSCacheMu.Unlock()

	return runtime, nil
}

func SelectCollectorForPBSRuntime(collectors []hubcollector.Collector, collectorID string) *hubcollector.Collector {
	collectorID = strings.TrimSpace(collectorID)

	if collectorID != "" {
		for i := range collectors {
			current := collectors[i]
			if current.CollectorType != hubcollector.CollectorTypePBS {
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
		if current.CollectorType != hubcollector.CollectorTypePBS {
			continue
		}
		return &current
	}

	return nil
}

func hasMultipleActivePBSCollectors(collectors []hubcollector.Collector) bool {
	count := 0
	for _, current := range collectors {
		if current.CollectorType != hubcollector.CollectorTypePBS {
			continue
		}
		count++
		if count > 1 {
			return true
		}
	}
	return false
}
