package collectors

import (
	"strings"

	"github.com/labtether/labtether/internal/modelmap"
)

func WithCanonicalResourceMetadata(source, assetType string, metadata map[string]string) (string, map[string]string) {
	out := cloneStringMap(metadata)
	if out == nil {
		out = make(map[string]string, 2)
	}

	resourceClass, resourceKind, _ := modelmap.DeriveAssetCanonical(source, assetType, out)
	if trimmed := strings.TrimSpace(resourceClass); trimmed != "" {
		out["resource_class"] = trimmed
	}
	if trimmed := strings.TrimSpace(resourceKind); trimmed != "" {
		out["resource_kind"] = trimmed
	}

	return strings.TrimSpace(resourceKind), out
}
