package pbs

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/servicehttp"
)

var (
	ErrPBSAssetNotFound = errors.New("pbs asset not found")
	ErrAssetNotPBS      = errors.New("asset is not pbs-backed")
)

func WritePBSResolveError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrPBSAssetNotFound), errors.Is(err, ErrAssetNotPBS):
		servicehttp.WriteError(w, http.StatusNotFound, err.Error())
	default:
		servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
	}
}

func (d *Deps) ResolvePBSAsset(assetID string) (assets.Asset, error) {
	if d.AssetStore == nil {
		return assets.Asset{}, ErrPBSAssetNotFound
	}

	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return assets.Asset{}, ErrPBSAssetNotFound
	}

	asset, ok, err := d.AssetStore.GetAsset(assetID)
	if err != nil {
		return assets.Asset{}, fmt.Errorf("failed to load asset: %w", err)
	}
	if !ok {
		return assets.Asset{}, ErrPBSAssetNotFound
	}
	if !strings.EqualFold(strings.TrimSpace(asset.Source), "pbs") {
		return assets.Asset{}, ErrAssetNotPBS
	}
	return asset, nil
}

func (d *Deps) ResolvePBSAssetRuntime(assetID string) (assets.Asset, *PBSRuntime, error) {
	asset, err := d.ResolvePBSAsset(assetID)
	if err != nil {
		return assets.Asset{}, nil, err
	}

	preferredCollectorID := strings.TrimSpace(asset.Metadata["collector_id"])
	runtime, loadErr := d.LoadPBSRuntime(preferredCollectorID)
	if loadErr != nil && preferredCollectorID != "" {
		runtime, loadErr = d.LoadPBSRuntime("")
	}
	if loadErr != nil {
		return assets.Asset{}, nil, fmt.Errorf("failed to load pbs runtime: %w", loadErr)
	}
	return asset, runtime, nil
}

func PBSNodeFromAsset(asset assets.Asset) string {
	if node := strings.TrimSpace(asset.Metadata["node"]); node != "" {
		return node
	}
	return "localhost"
}

func PBSStoreFromAsset(asset assets.Asset) string {
	if store := strings.TrimSpace(asset.Metadata["store"]); store != "" {
		return store
	}
	if strings.HasPrefix(strings.TrimSpace(asset.ID), "pbs-datastore-") {
		return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(asset.ID), "pbs-datastore-"))
	}
	return ""
}
