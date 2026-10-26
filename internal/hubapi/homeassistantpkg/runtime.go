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

	collectorID := strings.TrimSpace(requestedCollectorID)
	metadataOwner := strings.TrimSpace(asset.Metadata["collector_id"])
	if metadataOwner != "" {
		if collectorID != "" && collectorID != metadataOwner {
			return nil, fmt.Errorf("collector_id does not own the selected entity")
		}
		collectorID = metadataOwner
	}
	selected, err := shared.ResolveEnabledHubCollector(d.HubCollectorStore, hubcollector.CollectorTypeHomeAssistant, collectorID)
	if err != nil {
		return nil, err
	}
	if scope, scoped := assetid.CollectorScopeFromAssetID(asset.ID); scoped {
		if scope != assetid.CollectorScope(selected.ID) {
			return nil, fmt.Errorf("home assistant entity namespace conflicts with collector")
		}
	} else if metadataOwner == "" && collectorID != "" {
		// A request parameter cannot assign a genuinely unowned legacy entity
		// to an arbitrary provider when more than one is enabled.
		unique, err := shared.ResolveEnabledHubCollector(d.HubCollectorStore, hubcollector.CollectorTypeHomeAssistant, "")
		if err != nil || unique.ID != selected.ID {
			return nil, fmt.Errorf("home assistant legacy entity ownership is ambiguous")
		}
	}

	return d.loadRuntimeForCollector(selected)
}

// Callers resolve and validate the persisted collector before constructing its
// private client. Targetless actions use the same credential owner as entities.
func (d *Deps) loadRuntimeForCollector(selected *hubcollector.Collector) (*runtime, error) {
	if selected == nil || d.CredentialStore == nil || d.SecretsManager == nil {
		return nil, fmt.Errorf("home assistant credential store unavailable")
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
