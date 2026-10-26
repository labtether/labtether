package persistence

import (
	"context"

	"github.com/labtether/labtether/internal/assets"
)

// AssetContextStore is the optional cancellable exact asset lookup capability.
type AssetContextStore interface {
	GetAssetContext(context.Context, string) (assets.Asset, bool, error)
}

// WebServiceIdentityContextStore reads one current manual service without using
// the coordinator's presentation cache or a background-context fallback.
type WebServiceIdentityContextStore interface {
	GetManualWebServiceContext(context.Context, string) (WebServiceManual, bool, error)
}
