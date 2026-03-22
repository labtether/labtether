package persistence

import (
	"encoding/json"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/modelmap"
)

type assetScanner interface {
	Scan(dest ...any) error
}

func scanAsset(row assetScanner) (assets.Asset, error) {
	asset := assets.Asset{}
	var groupID *string
	var host *string
	var transportType *string
	var metadata []byte
	var tags []byte

	err := row.Scan(
		&asset.ID,
		&asset.Type,
		&asset.Name,
		&asset.Source,
		&groupID,
		&asset.Status,
		&asset.Platform,
		&metadata,
		&tags,
		&asset.CreatedAt,
		&asset.UpdatedAt,
		&asset.LastSeenAt,
		&host,
		&transportType,
	)
	if err != nil {
		return assets.Asset{}, err
	}
	if groupID != nil {
		asset.GroupID = *groupID
	}
	if host != nil {
		asset.Host = *host
	}
	if transportType != nil {
		asset.TransportType = *transportType
	}

	if len(metadata) > 0 {
		parsed := map[string]string{}
		if err := json.Unmarshal(metadata, &parsed); err == nil {
			asset.Metadata = parsed
		}
	}
	if len(tags) > 0 {
		parsed := []string{}
		if err := json.Unmarshal(tags, &parsed); err == nil {
			asset.Tags = assets.NormalizeTags(parsed)
		}
	}
	asset.Tags = cloneStringSlice(asset.Tags)
	asset.Metadata = cloneMetadata(asset.Metadata)
	asset.ResourceClass, asset.ResourceKind, asset.Attributes = modelmap.DeriveAssetCanonical(asset.Source, asset.Type, asset.Metadata)
	asset.Attributes = cloneAnyMap(asset.Attributes)

	return asset, nil
}

type credentialProfileScanner interface {
	Scan(dest ...any) error
}

func scanCredentialProfile(row credentialProfileScanner) (credentials.Profile, error) {
	profile := credentials.Profile{}
	var username *string
	var description *string
	var metadata []byte
	var passphraseCiphertext *string
	var rotatedAt *time.Time
	var lastUsedAt *time.Time
	var expiresAt *time.Time
	if err := row.Scan(
		&profile.ID,
		&profile.Name,
		&profile.Kind,
		&username,
		&description,
		&profile.Status,
		&metadata,
		&profile.SecretCiphertext,
		&passphraseCiphertext,
		&profile.CreatedAt,
		&profile.UpdatedAt,
		&rotatedAt,
		&lastUsedAt,
		&expiresAt,
	); err != nil {
		return credentials.Profile{}, err
	}

	if username != nil {
		profile.Username = *username
	}
	if description != nil {
		profile.Description = *description
	}
	if passphraseCiphertext != nil {
		profile.PassphraseCiphertext = *passphraseCiphertext
	}
	if rotatedAt != nil {
		value := rotatedAt.UTC()
		profile.RotatedAt = &value
	}
	if lastUsedAt != nil {
		value := lastUsedAt.UTC()
		profile.LastUsedAt = &value
	}
	if expiresAt != nil {
		value := expiresAt.UTC()
		profile.ExpiresAt = &value
	}
	if len(metadata) > 0 {
		parsed := map[string]string{}
		if err := json.Unmarshal(metadata, &parsed); err == nil {
			profile.Metadata = parsed
		}
	}
	profile.Metadata = cloneMetadata(profile.Metadata)
	return profile, nil
}

type assetTerminalConfigScanner interface {
	Scan(dest ...any) error
}

func scanAssetTerminalConfig(row assetTerminalConfigScanner) (credentials.AssetTerminalConfig, error) {
	cfg := credentials.AssetTerminalConfig{}
	var username *string
	var hostKey *string
	var profileID *string
	if err := row.Scan(
		&cfg.AssetID,
		&cfg.Host,
		&cfg.Port,
		&username,
		&cfg.StrictHostKey,
		&hostKey,
		&profileID,
		&cfg.UpdatedAt,
	); err != nil {
		return credentials.AssetTerminalConfig{}, err
	}
	if username != nil {
		cfg.Username = *username
	}
	if hostKey != nil {
		cfg.HostKey = *hostKey
	}
	if profileID != nil {
		cfg.CredentialProfileID = *profileID
	}
	cfg.UpdatedAt = cfg.UpdatedAt.UTC()
	return cfg, nil
}
