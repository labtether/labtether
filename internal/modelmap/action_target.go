package modelmap

import (
	"maps"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/assets"
)

// TrueNASVMActionParams maps a stored canonical VM to the provider's numeric
// target without replacing its canonical asset ID. Callers decide whether an
// explicit provider override is allowed before using these authoritative params.
// It leaves the input map unchanged and returns false when no mapping is known.
func TrueNASVMActionParams(actionID string, asset assets.Asset, params map[string]string) (map[string]string, bool) {
	if (actionID != "vm.start" && actionID != "vm.stop") ||
		!strings.EqualFold(strings.TrimSpace(asset.Source), "truenas") || !strings.EqualFold(asset.Type, "vm") {
		return params, false
	}
	vmID, err := strconv.Atoi(strings.TrimSpace(asset.Metadata["vm_id"]))
	if err != nil || vmID <= 0 {
		return params, false
	}
	normalized := maps.Clone(params)
	if normalized == nil {
		normalized = make(map[string]string)
	}
	normalized["vm_id"] = strconv.Itoa(vmID)
	return normalized, true
}
