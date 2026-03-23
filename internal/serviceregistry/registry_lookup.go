package serviceregistry

import "sort"

// LookupByDockerImage finds a known service by Docker image reference.
// The image is normalized (tag stripped, docker.io prefix removed, lowercased) before lookup.
func LookupByDockerImage(image string) (KnownService, bool) {
	normalized := normalizeDockerImage(image)
	if normalized == "" {
		return KnownService{}, false
	}
	if key, ok := imageIndex[normalized]; ok {
		return registry[key], true
	}
	return KnownService{}, false
}

// LookupByHint finds a known service from a name/domain/container hint.
// It is designed for fallback classification when image-based matching is unavailable.
func LookupByHint(hint string) (KnownService, bool) {
	seen := make(map[string]struct{}, 8)
	for _, candidate := range hintCandidates(hint) {
		normalized := normalizeServiceHint(candidate)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		if key, ok := hintIndex[normalized]; ok {
			return registry[key], true
		}
	}
	return KnownService{}, false
}

// LookupByPort finds a known service by its default port number.
// When multiple services share a port, the first registered service wins.
func LookupByPort(port int) (KnownService, bool) {
	if key, ok := portIndex[port]; ok {
		return registry[key], true
	}
	return KnownService{}, false
}

// LookupUniqueByPort finds a known service by default port only when the mapping is unique.
// Shared/ambiguous ports return not found.
func LookupUniqueByPort(port int) (KnownService, bool) {
	if key, ok := uniquePortIndex[port]; ok {
		return registry[key], true
	}
	return KnownService{}, false
}

// LookupByKey finds a known service by its unique key.
func LookupByKey(key string) (KnownService, bool) {
	svc, ok := registry[key]
	return svc, ok
}

// AllCategories returns a sorted, deduplicated list of all service categories in the registry.
func AllCategories() []string {
	seen := map[string]bool{}
	for _, svc := range registry {
		seen[svc.Category] = true
	}
	cats := make([]string, 0, len(seen))
	for c := range seen {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	return cats
}
