package alerting

import (
	"fmt"
	"strings"
)

var DeprecatedCanonicalPredicateKeys = map[string]string{
	"kind":         "resource_kind",
	"type":         "resource_kind",
	"asset_type":   "resource_kind",
	"target_kind":  "resource_kind",
	"asset_kind":   "resource_kind",
	"target_type":  "resource_kind",
	"target_class": "resource_class",
}

func ValidateNoDeprecatedCanonicalPredicateKeys(values map[string]any, fieldName string) error {
	for rawKey := range values {
		key := strings.ToLower(strings.TrimSpace(rawKey))
		if key == "" {
			continue
		}
		if replacement, deprecated := DeprecatedCanonicalPredicateKeys[key]; deprecated {
			return fmt.Errorf("%s key %q is deprecated; use %q", fieldName, rawKey, replacement)
		}
	}
	return nil
}
