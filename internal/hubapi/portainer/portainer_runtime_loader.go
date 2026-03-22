package portainer

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	pnconnector "github.com/labtether/labtether/internal/connectors/portainer"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubcollector"
)

func (d *Deps) LoadPortainerRuntime(collectorID string) (*PortainerRuntime, error) {
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

	selected := SelectCollectorForPortainerRuntime(collectors, collectorID)
	if selected == nil {
		return nil, fmt.Errorf("no active portainer collector configured")
	}

	baseURL := shared.CollectorConfigString(selected.Config, "base_url")
	credentialID := shared.CollectorConfigString(selected.Config, "credential_id")
	if baseURL == "" || credentialID == "" {
		return nil, fmt.Errorf("portainer collector config is incomplete")
	}

	authMethod := strings.ToLower(strings.TrimSpace(shared.CollectorConfigString(selected.Config, "auth_method")))
	if authMethod == "" {
		authMethod = "api_key"
	}

	credential, ok, err := d.CredentialStore.GetCredentialProfile(credentialID)
	if err != nil || !ok {
		return nil, fmt.Errorf("portainer credential profile not found")
	}

	decryptedSecret, err := d.SecretsManager.DecryptString(credential.SecretCiphertext, credential.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt portainer credential")
	}

	skipVerify, hasSkip := shared.CollectorConfigBool(selected.Config, "skip_verify")
	if !hasSkip {
		skipVerify = false
	}
	timeout := shared.CollectorConfigDuration(selected.Config, "timeout", 15*time.Second)

	secretHash := fmt.Sprintf("%x", sha256.Sum256([]byte(decryptedSecret)))
	configKey := fmt.Sprintf("%s|%s|%s|%v|%s|%s",
		strings.TrimSpace(baseURL),
		strings.TrimSpace(credentialID),
		authMethod,
		skipVerify,
		timeout.String(),
		secretHash,
	)

	d.PortainerCacheMu.RLock()
	if cached, hit := d.PortainerCache[selected.ID]; hit && cached.ConfigKey == configKey {
		d.PortainerCacheMu.RUnlock()
		return cached.Runtime, nil
	}
	d.PortainerCacheMu.RUnlock()

	cfg := pnconnector.Config{
		BaseURL:    strings.TrimSpace(baseURL),
		SkipVerify: skipVerify,
		Timeout:    timeout,
	}
	if authMethod == "password" {
		cfg.Username = strings.TrimSpace(credential.Username)
		cfg.Password = strings.TrimSpace(decryptedSecret)
		if cfg.Username == "" {
			return nil, fmt.Errorf("portainer password auth requires a username")
		}
		if cfg.Password == "" {
			return nil, fmt.Errorf("portainer password auth requires a password")
		}
	} else {
		cfg.APIKey = strings.TrimSpace(decryptedSecret)
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("portainer api_key auth requires an api key")
		}
	}

	client := pnconnector.NewClient(cfg)

	runtime := &PortainerRuntime{
		Client:      client,
		CollectorID: strings.TrimSpace(selected.ID),
		BaseURL:     strings.TrimSpace(baseURL),
		AuthMethod:  authMethod,
		SkipVerify:  skipVerify,
		Timeout:     timeout,
		ConfigKey:   configKey,
	}

	d.PortainerCacheMu.Lock()
	if d.PortainerCache == nil {
		d.PortainerCache = make(map[string]*CachedPortainerRuntime)
	}
	if len(d.PortainerCache) >= shared.MaxRuntimeCacheEntries {
		d.PortainerCache = make(map[string]*CachedPortainerRuntime)
	}
	d.PortainerCache[selected.ID] = &CachedPortainerRuntime{
		Runtime:   runtime,
		ConfigKey: configKey,
	}
	d.PortainerCacheMu.Unlock()

	return runtime, nil
}

func SelectCollectorForPortainerRuntime(collectors []hubcollector.Collector, collectorID string) *hubcollector.Collector {
	collectorID = strings.TrimSpace(collectorID)

	if collectorID != "" {
		for i := range collectors {
			current := collectors[i]
			if current.CollectorType != hubcollector.CollectorTypePortainer {
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
		if current.CollectorType != hubcollector.CollectorTypePortainer {
			continue
		}
		return &current
	}

	return nil
}
