package truenas

import (
	"path"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/hubapi/shared"
)

// MaxTrueNASCacheEntries is the upper bound for each in-memory TrueNAS read
// cache map. When exceeded the entire map is cleared to prevent unbounded
// growth caused by large numbers of unique asset/collector IDs.
const MaxTrueNASCacheEntries = 1000

func TrueNASSmartAssetCacheKey(assetID string) string {
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return ""
	}
	return "asset:" + assetID
}

func TrueNASSmartCollectorCacheKey(collectorID string) string {
	collectorID = strings.TrimSpace(collectorID)
	if collectorID == "" {
		return ""
	}
	return "collector:" + collectorID
}

func (d *Deps) GetCachedTrueNASSMART(assetID, collectorID string) (TrueNASAssetSMARTResponse, bool) {
	assetKey := TrueNASSmartAssetCacheKey(assetID)
	collectorKey := TrueNASSmartCollectorCacheKey(collectorID)
	if assetKey == "" && collectorKey == "" {
		return TrueNASAssetSMARTResponse{}, false
	}

	d.TruenasReadCacheMu.RLock()
	defer d.TruenasReadCacheMu.RUnlock()
	if d.TruenasSmartCache == nil {
		return TrueNASAssetSMARTResponse{}, false
	}

	if assetKey != "" {
		if cached, ok := d.TruenasSmartCache[assetKey]; ok {
			return cached, true
		}
	}
	if collectorKey != "" {
		if cached, ok := d.TruenasSmartCache[collectorKey]; ok {
			return cached, true
		}
	}
	return TrueNASAssetSMARTResponse{}, false
}

func (d *Deps) SetCachedTrueNASSMART(assetID, collectorID string, response TrueNASAssetSMARTResponse) {
	assetKey := TrueNASSmartAssetCacheKey(assetID)
	collectorKey := TrueNASSmartCollectorCacheKey(collectorID)
	if assetKey == "" && collectorKey == "" {
		return
	}

	d.TruenasReadCacheMu.Lock()
	if d.TruenasSmartCache == nil {
		d.TruenasSmartCache = make(map[string]TrueNASAssetSMARTResponse)
	}
	if len(d.TruenasSmartCache) >= MaxTrueNASCacheEntries {
		d.TruenasSmartCache = make(map[string]TrueNASAssetSMARTResponse)
	}
	if assetKey != "" {
		d.TruenasSmartCache[assetKey] = response
	}
	if collectorKey != "" {
		d.TruenasSmartCache[collectorKey] = response
	}
	d.TruenasReadCacheMu.Unlock()
}

func TrueNASFilesystemCacheKey(scope, id, requestPath string) string {
	scope = strings.TrimSpace(scope)
	id = strings.TrimSpace(id)
	if scope == "" || id == "" {
		return ""
	}
	return scope + ":" + id + "|" + NormalizeTrueNASFilesystemPath(requestPath)
}

func (d *Deps) GetCachedTrueNASFilesystem(assetID, collectorID, requestPath string) (TrueNASFilesystemResponse, bool) {
	assetKey := TrueNASFilesystemCacheKey("asset", assetID, requestPath)
	collectorKey := TrueNASFilesystemCacheKey("collector", collectorID, requestPath)
	if assetKey == "" && collectorKey == "" {
		return TrueNASFilesystemResponse{}, false
	}

	d.TruenasReadCacheMu.RLock()
	defer d.TruenasReadCacheMu.RUnlock()
	if d.TruenasFSCache == nil {
		return TrueNASFilesystemResponse{}, false
	}

	if assetKey != "" {
		if cached, ok := d.TruenasFSCache[assetKey]; ok {
			return cached, true
		}
	}
	if collectorKey != "" {
		if cached, ok := d.TruenasFSCache[collectorKey]; ok {
			return cached, true
		}
	}
	return TrueNASFilesystemResponse{}, false
}

func (d *Deps) SetCachedTrueNASFilesystem(assetID, collectorID, requestPath string, response TrueNASFilesystemResponse) {
	assetKey := TrueNASFilesystemCacheKey("asset", assetID, requestPath)
	collectorKey := TrueNASFilesystemCacheKey("collector", collectorID, requestPath)
	if assetKey == "" && collectorKey == "" {
		return
	}

	d.TruenasReadCacheMu.Lock()
	if d.TruenasFSCache == nil {
		d.TruenasFSCache = make(map[string]TrueNASFilesystemResponse)
	}
	if len(d.TruenasFSCache) >= MaxTrueNASCacheEntries {
		d.TruenasFSCache = make(map[string]TrueNASFilesystemResponse)
	}
	if assetKey != "" {
		d.TruenasFSCache[assetKey] = response
	}
	if collectorKey != "" {
		d.TruenasFSCache[collectorKey] = response
	}
	d.TruenasReadCacheMu.Unlock()
}

func NormalizeTrueNASListDirResult(result any) ([]map[string]any, bool) {
	switch typed := result.(type) {
	case []any:
		entries := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			entries = append(entries, entry)
		}
		return entries, true
	case map[string]any:
		if data, ok := typed["entries"]; ok {
			return NormalizeTrueNASListDirResult(data)
		}
		if data, ok := typed["data"]; ok {
			return NormalizeTrueNASListDirResult(data)
		}
	}
	return nil, false
}

func MapTrueNASFilesystemEntry(entry map[string]any, basePath string) TrueNASFilesystemEntry {
	name := strings.TrimSpace(shared.CollectorAnyString(entry["name"]))
	entryPath := strings.TrimSpace(shared.CollectorAnyString(entry["path"]))
	if entryPath == "" {
		if name == "" {
			entryPath = NormalizeTrueNASFilesystemPath(basePath)
		} else {
			entryPath = NormalizeTrueNASFilesystemPath(path.Join(basePath, name))
		}
	}
	if name == "" {
		name = path.Base(strings.TrimRight(entryPath, "/"))
	}
	if name == "." || name == "/" {
		name = entryPath
	}

	entryType := strings.ToLower(strings.TrimSpace(shared.CollectorAnyString(entry["type"])))
	isDir := entryType == "directory" || entryType == "dir"
	if !isDir {
		if parsed, ok := shared.ParseAnyBoolLoose(entry["is_dir"]); ok {
			isDir = parsed
		}
	}
	if entryType == "" {
		if isDir {
			entryType = "directory"
		} else {
			entryType = "file"
		}
	}

	var sizeBytes *int64
	if parsed, ok := shared.ParseAnyInt64(entry["size"]); ok && parsed >= 0 {
		sizeBytes = &parsed
	}

	modifiedAt := ""
	if modifiedRaw := entry["mtime"]; modifiedRaw != nil {
		if parsed, ok := shared.ParseAnyTimestamp(modifiedRaw); ok {
			modifiedAt = parsed.UTC().Format(time.RFC3339)
		}
	}
	if modifiedAt == "" {
		if modifiedRaw := entry["modified"]; modifiedRaw != nil {
			if parsed, ok := shared.ParseAnyTimestamp(modifiedRaw); ok {
				modifiedAt = parsed.UTC().Format(time.RFC3339)
			}
		}
	}

	isSymbolic := false
	if parsed, ok := shared.ParseAnyBoolLoose(entry["is_symlink"]); ok {
		isSymbolic = parsed
	}
	if entryType == "symlink" {
		isSymbolic = true
	}

	return TrueNASFilesystemEntry{
		Name:         name,
		Path:         entryPath,
		Type:         entryType,
		SizeBytes:    sizeBytes,
		Mode:         strings.TrimSpace(shared.CollectorAnyString(entry["mode"])),
		ModifiedAt:   modifiedAt,
		User:         strings.TrimSpace(shared.CollectorAnyString(entry["user"])),
		Group:        strings.TrimSpace(shared.CollectorAnyString(entry["group"])),
		IsDirectory:  isDir,
		IsSymbolic:   isSymbolic,
		SymbolicLink: strings.TrimSpace(shared.CollectorAnyString(entry["realpath"])),
	}
}

// invalidateTrueNASCaches clears all in-memory read-cache entries for the
// given asset/collector pair. It must be called after any mutating action so
// that the next read returns fresh data rather than a stale cached response.
//
// The SMART cache is keyed by asset ID and collector ID; the filesystem cache
// is keyed by (scope, id, path) but the path component does not matter here
// because a mutating operation may affect any path — the entire map is
// cleared for this asset/collector by iterating and deleting matching
// prefixes.
func (d *Deps) InvalidateTrueNASCaches(assetID, collectorID string) {
	assetKey := TrueNASSmartAssetCacheKey(assetID)
	collectorKey := TrueNASSmartCollectorCacheKey(collectorID)

	d.TruenasReadCacheMu.Lock()
	defer d.TruenasReadCacheMu.Unlock()

	// Clear SMART cache entries for this asset/collector.
	if d.TruenasSmartCache != nil {
		if assetKey != "" {
			delete(d.TruenasSmartCache, assetKey)
		}
		if collectorKey != "" {
			delete(d.TruenasSmartCache, collectorKey)
		}
	}

	// Clear filesystem cache entries whose key prefix matches this asset or
	// collector (the key format is "asset:<id>|<path>" or "collector:<id>|<path>").
	if d.TruenasFSCache != nil {
		assetPrefix := ""
		if assetID != "" {
			assetPrefix = "asset:" + strings.TrimSpace(assetID) + "|"
		}
		collectorPrefix := ""
		if collectorID != "" {
			collectorPrefix = "collector:" + strings.TrimSpace(collectorID) + "|"
		}
		for k := range d.TruenasFSCache {
			if (assetPrefix != "" && strings.HasPrefix(k, assetPrefix)) ||
				(collectorPrefix != "" && strings.HasPrefix(k, collectorPrefix)) {
				delete(d.TruenasFSCache, k)
			}
		}
	}
}

