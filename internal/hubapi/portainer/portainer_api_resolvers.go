package portainer

import (
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/servicehttp"
)

var (
	ErrPortainerAssetNotFound = errors.New("portainer asset not found")
	ErrAssetNotPortainer      = errors.New("asset is not portainer-backed")
)

func writePortainerResolveError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrPortainerAssetNotFound), errors.Is(err, ErrAssetNotPortainer):
		servicehttp.WriteError(w, http.StatusNotFound, err.Error())
	default:
		servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
	}
}

func (d *Deps) ResolvePortainerAsset(assetID string) (assets.Asset, error) {
	if d.AssetStore == nil {
		return assets.Asset{}, ErrPortainerAssetNotFound
	}

	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return assets.Asset{}, ErrPortainerAssetNotFound
	}

	asset, ok, err := d.AssetStore.GetAsset(assetID)
	if err != nil {
		return assets.Asset{}, fmt.Errorf("failed to load asset: %w", err)
	}
	if !ok {
		return assets.Asset{}, ErrPortainerAssetNotFound
	}
	if !strings.EqualFold(strings.TrimSpace(asset.Source), "portainer") {
		return assets.Asset{}, ErrAssetNotPortainer
	}
	return asset, nil
}

func (d *Deps) ResolvePortainerAssetRuntime(assetID string) (assets.Asset, *PortainerRuntime, error) {
	asset, err := d.ResolvePortainerAsset(assetID)
	if err != nil {
		return assets.Asset{}, nil, err
	}

	preferredCollectorID := strings.TrimSpace(asset.Metadata["collector_id"])
	runtime, loadErr := d.LoadPortainerRuntime(preferredCollectorID)
	if loadErr != nil && preferredCollectorID != "" {
		runtime, loadErr = d.LoadPortainerRuntime("")
	}
	if loadErr != nil {
		return assets.Asset{}, nil, fmt.Errorf("failed to load portainer runtime: %w", loadErr)
	}
	return asset, runtime, nil
}
