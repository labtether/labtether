package homeassistantpkg

import (
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/homeassistant"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubcollector"
)

type runtime struct {
	Connector   *homeassistant.Connector
	CollectorID string
}

func (d *Deps) loadRuntimeForAsset(asset assets.Asset, requestedCollectorID string) (*runtime, error) {
	if d.HubCollectorStore == nil {
		return nil, fmt.Errorf("home assistant collector store unavailable")
	}
	if d.CredentialStore == nil || d.SecretsManager == nil {
		return nil, fmt.Errorf("home assistant credential store unavailable")
	}

	collectors, err := d.HubCollectorStore.ListHubCollectors(200, true)
	if err != nil {
		return nil, fmt.Errorf("failed to list home assistant collectors: %w", err)
	}
	collectorID := strings.TrimSpace(requestedCollectorID)
	if metadataCollectorID := strings.TrimSpace(asset.Metadata["collector_id"]); metadataCollectorID != "" {
		if collectorID != "" && collectorID != metadataCollectorID {
			return nil, fmt.Errorf("collector_id does not own the selected entity")
		}
		collectorID = metadataCollectorID
	}
	if collectorID == "" {
		if scope, scoped := assetid.CollectorScopeFromAssetID(asset.ID); scoped {
			for _, candidate := range collectors {
				if candidate.CollectorType == hubcollector.CollectorTypeHomeAssistant && assetid.CollectorScope(candidate.ID) == scope {
					collectorID = candidate.ID
					break
				}
			}
		}
	}

	var selected *hubcollector.Collector
	for i := range collectors {
		candidate := collectors[i]
		if hubcollector.NormalizeCollectorType(candidate.CollectorType) != hubcollector.CollectorTypeHomeAssistant {
			continue
		}
		if collectorID != "" && candidate.ID != collectorID {
			continue
		}
		if selected != nil && collectorID == "" {
			return nil, fmt.Errorf("multiple active home assistant collectors are configured; collector-aware entity identity is required")
		}
		selected = &candidate
	}
	if selected == nil {
		return nil, fmt.Errorf("no active home assistant collector configured for entity")
	}

	baseURL := shared.CollectorConfigString(selected.Config, "base_url")
	credentialID := shared.CollectorConfigString(selected.Config, "credential_id")
	if baseURL == "" || credentialID == "" {
		return nil, fmt.Errorf("home assistant collector config is incomplete")
	}
	credential, ok, err := d.CredentialStore.GetCredentialProfile(credentialID)
	if err != nil || !ok {
		return nil, fmt.Errorf("home assistant credential profile not found")
	}
	if credential.Kind != credentials.KindHomeAssistantToken {
		return nil, fmt.Errorf("home assistant collector credential kind is invalid")
	}
	token, err := d.SecretsManager.DecryptString(credential.SecretCiphertext, credential.ID)
	if err != nil || strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("failed to decrypt home assistant credential")
	}
	skipVerify, hasSkipVerify := shared.CollectorConfigBool(selected.Config, "skip_verify")
	if !hasSkipVerify {
		skipVerify = false
	}
	timeout := shared.CollectorConfigDuration(selected.Config, "timeout", 15*time.Second)

	return &runtime{
		Connector: homeassistant.NewWithConfig(homeassistant.Config{
			BaseURL:    baseURL,
			Token:      strings.TrimSpace(token),
			SkipVerify: skipVerify,
			Timeout:    timeout,
		}),
		CollectorID: selected.ID,
	}, nil
}
