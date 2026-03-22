package webservice

import (
	"sort"
	"strings"
)

func buildRegistryIndexes() {
	imageIndex = make(map[string]string, len(registry)*2)
	portIndex = make(map[int]string, len(registry))
	uniquePortIndex = make(map[int]string, len(registry))
	hintIndex = make(map[string]string, len(registry)*8)
	portCounts := make(map[int]int, len(registry))

	// Sort keys for deterministic index building (first alphabetically wins port conflicts).
	keys := make([]string, 0, len(registry))
	for key := range registry {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		svc := registry[key]
		// Build image index
		for _, img := range svc.DockerImages {
			normalized := normalizeDockerImage(img)
			if normalized == "" {
				continue
			}
			if _, exists := imageIndex[normalized]; !exists {
				imageIndex[normalized] = key
			}
		}

		// Build name/domain hint index.
		addHintAlias(key, key)
		addHintAlias(key, svc.Name)
		addHintAlias(key, svc.IconKey)
		for _, img := range svc.DockerImages {
			normalized := normalizeDockerImage(img)
			addHintAlias(key, normalized)
			if slash := strings.LastIndex(normalized, "/"); slash >= 0 && slash+1 < len(normalized) {
				addHintAlias(key, normalized[slash+1:])
			}
		}

		// Build port index (first key alphabetically wins for conflicts)
		if svc.DefaultPort > 0 {
			portCounts[svc.DefaultPort]++
			if _, exists := portIndex[svc.DefaultPort]; !exists {
				portIndex[svc.DefaultPort] = key
			}
		}
	}

	buildUniquePortIndex(portCounts)
}

func buildUniquePortIndex(portCounts map[int]int) {
	// Build unique port index (only ports used by exactly one service).
	for port, key := range portIndex {
		if portCounts[port] == 1 {
			uniquePortIndex[port] = key
		}
	}
}
