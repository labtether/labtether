package shared

import "strings"

// MaxRuntimeCacheEntries caps the size of in-memory runtime client caches
// (Proxmox, PBS, TrueNAS). When the limit is reached the entire cache is
// cleared so the next lookup rebuilds a fresh entry. In practice these maps
// hold one entry per configured collector, so the limit is generous.
const MaxRuntimeCacheEntries = 200

func CloneAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}

	out := make(map[string]any, len(input))
	for key, value := range input {
		out[strings.TrimSpace(key)] = value
	}
	return out
}
