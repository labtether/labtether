package portainer

import (
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/servicehttp"
)

var (
	ErrPortainerAssetNotFound = errors.New("device not found")
	ErrAssetNotPortainer      = errors.New("this is not a Portainer device")
)

func writePortainerResolveError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrPortainerAssetNotFound) || errors.Is(err, ErrAssetNotPortainer) {
		servicehttp.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	// A removed or turned-off connection is not a Portainer failure.
	if shared.WriteCollectorSetupError(w, err, "Portainer", true) {
		return
	}
	writePortainerUpstreamError(w, err)
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
	if loadErr != nil {
		return assets.Asset{}, nil, fmt.Errorf("failed to load portainer runtime: %w", loadErr)
	}
	// The namespace is an owner even if legacy metadata has lost its collector ID.
	if scope, scoped := assetid.CollectorScopeFromAssetID(asset.ID); scoped && scope != assetid.CollectorScope(runtime.CollectorID) {
		return assets.Asset{}, nil, fmt.Errorf("portainer asset collector does not match runtime")
	}
	return asset, runtime, nil
}

// writePortainerUpstreamError answers a Portainer request that failed after
// its connection was loaded. A Portainer that cannot be reached, or refuses
// the saved login, is said in fixed words; anything else stays a hidden 502.
func writePortainerUpstreamError(w http.ResponseWriter, err error) {
	if shared.WriteUpstreamUnreachable(w, err, "Portainer") {
		return
	}
	servicehttp.WriteError(w, http.StatusBadGateway, shared.SanitizeUpstreamError(err.Error()))
}
