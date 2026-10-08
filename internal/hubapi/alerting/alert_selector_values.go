package alerting

import (
	"fmt"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/model"
	"sort"
	"strconv"
	"strings"
)

func assetResourceKindCandidates(entry assets.Asset) map[string]struct{} {
	out := make(map[string]struct{}, 3)
	addKindCandidate := func(value string) {
		normalized := normalizeKindToken(value)
		if normalized == "" {
			return
		}
		out[normalized] = struct{}{}
	}

	addKindCandidate(entry.ResourceKind)
	addKindCandidate(entry.Metadata["resource_kind"])
	addKindCandidate(entry.Type)

	return out
}

func assetResourceClassCandidates(entry assets.Asset) map[string]struct{} {
	out := make(map[string]struct{}, 3)
	addClass := func(value string) {
		normalized := normalizeSelectorToken(value)
		if normalized == "" {
			return
		}
		out[normalized] = struct{}{}
	}

	addClass(entry.ResourceClass)
	addClass(entry.Metadata["resource_class"])

	derivedKind := normalizeKindToken(firstNonEmptyString(entry.ResourceKind, entry.Metadata["resource_kind"], entry.Type))
	if derivedKind != "" {
		derivedClass := string(model.ResourceClassForKind(derivedKind))
		addClass(derivedClass)
	}

	return out
}

func selectorStringValues(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}
		if strings.Contains(trimmed, ",") {
			parts := strings.Split(trimmed, ",")
			out := make([]string, 0, len(parts))
			for _, part := range parts {
				if normalized := strings.TrimSpace(part); normalized != "" {
					out = append(out, normalized)
				}
			}
			return out
		}
		return []string{trimmed}
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if normalized := strings.TrimSpace(item); normalized != "" {
				out = append(out, normalized)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, selectorStringValues(item)...)
		}
		return out
	case bool:
		if typed {
			return []string{"true"}
		}
		return []string{"false"}
	default:
		return []string{strings.TrimSpace(fmt.Sprint(typed))}
	}
}

func normalizeSelectorValues(values []string, normalizer func(string) string) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalizer(value)
		if normalized == "" {
			continue
		}
		if _, exists := set[normalized]; exists {
			continue
		}
		set[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func setContainsAny(set map[string]struct{}, expected []string) bool {
	if len(expected) == 0 {
		return false
	}
	for _, value := range expected {
		if _, ok := set[value]; ok {
			return true
		}
	}
	return false
}

func setContainsAll(set map[string]struct{}, expected []string) bool {
	if len(expected) == 0 {
		return false
	}
	for _, value := range expected {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}

func valueMatchesExpected(candidate string, expected []string) bool {
	normalizedCandidate := normalizeSelectorToken(candidate)
	if normalizedCandidate == "" {
		return false
	}
	for _, value := range normalizeSelectorValues(expected, normalizeSelectorToken) {
		if normalizedCandidate == value {
			return true
		}
	}
	return false
}

func attributePathMatchesExpected(attributes map[string]any, path string, expected []string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	rawValue, ok := readAttributePath(attributes, path)
	if !ok {
		return false
	}
	candidate := normalizeSelectorToken(fmt.Sprint(rawValue))
	if candidate == "" {
		return false
	}
	for _, value := range normalizeSelectorValues(expected, normalizeSelectorToken) {
		if value == candidate {
			return true
		}
	}
	return false
}

func readAttributePath(attributes map[string]any, path string) (any, bool) {
	if len(attributes) == 0 {
		return nil, false
	}
	segments := strings.Split(path, ".")
	current := any(attributes)
	for _, segment := range segments {
		key := strings.TrimSpace(segment)
		if key == "" {
			return nil, false
		}
		switch typed := current.(type) {
		case map[string]any:
			next, ok := typed[key]
			if !ok {
				return nil, false
			}
			current = next
		case map[string]string:
			next, ok := typed[key]
			if !ok {
				return nil, false
			}
			current = next
		case []any:
			index, err := strconv.Atoi(key)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, false
			}
			current = typed[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func normalizeSelectorToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeKindToken(value string) string {
	normalized := normalizeSelectorToken(value)
	if normalized == "" {
		return ""
	}
	normalized = strings.ReplaceAll(normalized, "_", "-")
	normalized = strings.ReplaceAll(normalized, " ", "-")
	normalized = alertKindTokenSanitizePattern.ReplaceAllString(normalized, "-")
	normalized = strings.Trim(normalized, "-")
	return normalized
}

func sortedAssetsByID(values map[string]assets.Asset) []assets.Asset {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]assets.Asset, 0, len(keys))
	for _, key := range keys {
		out = append(out, values[key])
	}
	return out
}
