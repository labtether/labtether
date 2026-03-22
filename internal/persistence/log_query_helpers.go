package persistence

import "strings"

func normalizeLogFieldKeys(input []string) []string {
	if len(input) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(input))
	out := make([]string, 0, len(input))
	for _, raw := range input {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func normalizeLogAssetIDs(input []string) []string {
	if len(input) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(input))
	out := make([]string, 0, len(input))
	for _, raw := range input {
		assetID := strings.TrimSpace(raw)
		if assetID == "" {
			continue
		}
		if _, ok := seen[assetID]; ok {
			continue
		}
		seen[assetID] = struct{}{}
		out = append(out, assetID)
	}
	return out
}

func projectLogFields(input map[string]string, fieldKeys []string) map[string]string {
	if len(fieldKeys) == 0 || len(input) == 0 {
		return nil
	}

	out := make(map[string]string, len(fieldKeys))
	for _, key := range fieldKeys {
		if value, ok := input[key]; ok {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
