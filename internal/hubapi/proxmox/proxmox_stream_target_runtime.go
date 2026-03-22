package proxmox

import (
	"crypto/sha256"
	"fmt"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/hubcollector"
)

func (d *Deps) ResolveProxmoxSessionTarget(assetID string) (ProxmoxSessionTarget, bool, error) {
	if d.AssetStore == nil {
		return ProxmoxSessionTarget{}, false, nil
	}
	asset, ok, err := d.AssetStore.GetAsset(strings.TrimSpace(assetID))
	if err != nil {
		return ProxmoxSessionTarget{}, false, err
	}
	if !ok {
		return ProxmoxSessionTarget{}, false, nil
	}
	if !strings.EqualFold(strings.TrimSpace(asset.Source), "proxmox") {
		return ProxmoxSessionTarget{}, false, nil
	}

	target := ProxmoxSessionTarget{
		Kind:        strings.ToLower(strings.TrimSpace(asset.Metadata["proxmox_type"])),
		Node:        strings.TrimSpace(asset.Metadata["node"]),
		VMID:        strings.TrimSpace(asset.Metadata["vmid"]),
		CollectorID: strings.TrimSpace(asset.Metadata["collector_id"]),
	}

	if target.Kind == "" {
		switch {
		case strings.HasPrefix(asset.ID, "proxmox-vm-"), strings.EqualFold(asset.Type, "vm"):
			target.Kind = "qemu"
		case strings.HasPrefix(asset.ID, "proxmox-ct-"), strings.EqualFold(asset.Type, "container"):
			target.Kind = "lxc"
		case strings.HasPrefix(asset.ID, "proxmox-storage-"), strings.EqualFold(asset.Type, "storage-pool"):
			target.Kind = "storage"
		default:
			target.Kind = "node"
		}
	}

	if target.VMID == "" {
		switch target.Kind {
		case "qemu":
			target.VMID = strings.TrimPrefix(strings.TrimSpace(asset.ID), "proxmox-vm-")
		case "lxc":
			target.VMID = strings.TrimPrefix(strings.TrimSpace(asset.ID), "proxmox-ct-")
		}
	}
	if target.Kind == "storage" {
		storageID := strings.TrimSpace(asset.Metadata["storage_id"])
		if storageID != "" {
			parts := strings.Split(storageID, "/")
			target.StorageName = parts[len(parts)-1]
			if target.Node == "" {
				if len(parts) >= 3 && strings.EqualFold(strings.TrimSpace(parts[0]), "storage") {
					target.Node = strings.TrimSpace(parts[1])
				} else if len(parts) >= 2 {
					target.Node = strings.TrimSpace(parts[0])
				}
			}
		}
	}
	if target.Node == "" {
		name := strings.TrimSpace(asset.Name)
		if target.Kind == "storage" && name != "" {
			parts := strings.Split(name, "/")
			if len(parts) >= 3 && strings.EqualFold(strings.TrimSpace(parts[0]), "storage") {
				target.Node = strings.TrimSpace(parts[1])
				if target.StorageName == "" {
					target.StorageName = strings.TrimSpace(parts[len(parts)-1])
				}
			} else if len(parts) >= 2 {
				target.Node = strings.TrimSpace(parts[0])
				if target.StorageName == "" {
					target.StorageName = strings.TrimSpace(parts[len(parts)-1])
				}
			}
		}
		if target.Node == "" {
			target.Node = name
		}
	}
	if target.Node == "" {
		return ProxmoxSessionTarget{}, true, fmt.Errorf("proxmox asset missing node metadata")
	}
	if target.Kind != "node" && target.Kind != "storage" && target.VMID == "" {
		return ProxmoxSessionTarget{}, true, fmt.Errorf("proxmox asset missing vmid metadata")
	}
	return target, true, nil
}

func (d *Deps) LoadProxmoxRuntime(collectorID string) (*ProxmoxRuntime, error) {
	if d.HubCollectorStore == nil {
		return nil, fmt.Errorf("hub collector store unavailable")
	}
	if d.CredentialStore == nil || d.SecretsManager == nil {
		return nil, fmt.Errorf("credential store unavailable")
	}

	var selected *hubcollector.Collector
	if collectorID != "" {
		col, ok, getErr := d.HubCollectorStore.GetHubCollector(collectorID)
		if getErr != nil {
			return nil, fmt.Errorf("failed to load hub collector: %w", getErr)
		}
		if ok && col.CollectorType == hubcollector.CollectorTypeProxmox && col.Enabled {
			selected = &col
		}
	} else {
		collectors, err := d.HubCollectorStore.ListHubCollectors(200, true)
		if err != nil {
			return nil, fmt.Errorf("failed to list hub collectors: %w", err)
		}
		for i := range collectors {
			current := collectors[i]
			if current.CollectorType != hubcollector.CollectorTypeProxmox {
				continue
			}
			selected = &current
			break
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("no active proxmox collector configured")
	}

	baseURL := shared.CollectorConfigString(selected.Config, "base_url")
	credentialID := shared.CollectorConfigString(selected.Config, "credential_id")
	if baseURL == "" || credentialID == "" {
		return nil, fmt.Errorf("proxmox collector config is incomplete")
	}

	credential, ok, err := d.CredentialStore.GetCredentialProfile(credentialID)
	if err != nil || !ok {
		return nil, fmt.Errorf("proxmox credential profile not found")
	}

	skipVerify, hasSkip := shared.CollectorConfigBool(selected.Config, "skip_verify")
	if !hasSkip {
		skipVerify = false
	}
	caPEM := shared.CollectorConfigString(selected.Config, "ca_pem")

	// Determine auth mode from config. API tokens remain the default.
	authMethod := shared.CollectorConfigString(selected.Config, "auth_method")
	authMode := proxmox.AuthModeAPIToken
	if authMethod == "password" {
		authMode = proxmox.AuthModePassword
	}

	var tokenID, username string
	if authMode == proxmox.AuthModePassword {
		username = shared.CollectorConfigString(selected.Config, "username")
		if username == "" {
			username = strings.TrimSpace(credential.Username)
		}
		if username == "" {
			return nil, fmt.Errorf("proxmox username missing in credential profile")
		}
	} else {
		tokenID = shared.CollectorConfigString(selected.Config, "token_id")
		if tokenID == "" {
			tokenID = strings.TrimSpace(credential.Username)
		}
		if tokenID == "" {
			tokenID = strings.TrimSpace(credential.Metadata["token_id"])
		}
		if tokenID == "" {
			return nil, fmt.Errorf("proxmox token_id missing in collector config")
		}
	}

	secretHash := fmt.Sprintf("%x", sha256.Sum256([]byte(credential.SecretCiphertext)))
	configKey := fmt.Sprintf("%s|%s|%s|%s|%v|%s|%s", authMode, baseURL, tokenID+username, credentialID, skipVerify, caPEM, secretHash)

	// Check cache under read lock.
	d.ProxmoxCacheMu.RLock()
	if cached, hit := d.ProxmoxCache[selected.ID]; hit && cached.configKey == configKey {
		d.ProxmoxCacheMu.RUnlock()
		return cached.runtime, nil
	}
	d.ProxmoxCacheMu.RUnlock()

	decryptedSecret, err := d.SecretsManager.DecryptString(credential.SecretCiphertext, credential.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt proxmox credential")
	}

	tokenSecret := ""
	password := ""
	if authMode == proxmox.AuthModePassword {
		password = decryptedSecret
	} else {
		tokenSecret = decryptedSecret
	}

	// Cache miss or config changed — build new client.
	client, err := proxmox.NewClient(proxmox.Config{
		BaseURL:     baseURL,
		TokenID:     tokenID,
		TokenSecret: tokenSecret,
		SkipVerify:  skipVerify,
		CAPEM:       caPEM,
		Timeout:     shared.CollectorConfigDuration(selected.Config, "timeout", 15*time.Second),
		AuthMode:    authMode,
		Username:    username,
		Password:    password,
	})
	if err != nil {
		return nil, err
	}

	runtime := &ProxmoxRuntime{
		client:      client,
		authMode:    authMode,
		tokenID:     tokenID,
		tokenSecret: tokenSecret,
		skipVerify:  skipVerify,
		caPEM:       caPEM,
		collectorID: selected.ID,
	}

	// Store in cache under write lock.
	d.ProxmoxCacheMu.Lock()
	if d.ProxmoxCache == nil {
		d.ProxmoxCache = make(map[string]*CachedProxmoxRuntime)
	}
	if len(d.ProxmoxCache) >= shared.MaxRuntimeCacheEntries {
		d.ProxmoxCache = make(map[string]*CachedProxmoxRuntime)
	}
	d.ProxmoxCache[selected.ID] = &CachedProxmoxRuntime{
		runtime:   runtime,
		configKey: configKey,
	}
	d.ProxmoxCacheMu.Unlock()

	return runtime, nil
}

// tryProxmoxTerminalStream attempts to establish a Proxmox terminal proxy
// connection. It returns an error if the connection fails before the browser
// WebSocket is upgraded (allowing the caller to fall through to SSH). Once
// the browser WebSocket is upgraded, errors are handled internally and nil
