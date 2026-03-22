package resources

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) HandleMetricsOverview(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/metrics/overview" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	window := parseDurationParam(r.URL.Query().Get("window"), time.Hour, 5*time.Minute, 24*time.Hour)
	now := time.Now().UTC()
	if d.AssetStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "asset store unavailable")
		return
	}
	if d.TelemetryStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "telemetry store unavailable")
		return
	}

	assetList, err := d.AssetStore.ListAssets()
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list assets")
		return
	}

	if groupID := shared.GroupIDQueryParam(r); groupID != "" {
		if d.GroupStore == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "group store unavailable")
			return
		}
		_, ok, err := d.GroupStore.GetGroup(groupID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load group")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "group not found")
			return
		}

		filtered := make([]assets.Asset, 0, len(assetList))
		for _, assetEntry := range assetList {
			if strings.TrimSpace(assetEntry.GroupID) == groupID {
				filtered = append(filtered, assetEntry)
			}
		}
		assetList = filtered
	}

	out := make([]shared.AssetTelemetryOverview, 0, len(assetList))

	// Prefer the dynamic batch path when the store supports it.
	if dynStore, ok := d.TelemetryStore.(persistence.TelemetryDynamicStore); ok {
		assetIDs := make([]string, 0, len(assetList))
		for _, assetEntry := range assetList {
			assetIDs = append(assetIDs, assetEntry.ID)
		}

		dynSnapshots, err := dynStore.DynamicSnapshotMany(assetIDs, now)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to query telemetry")
			return
		}

		for _, assetEntry := range assetList {
			// Zero-value DynamicSnapshot (nil Metrics map) is safe here — map reads
		// on nil maps return zero values, and ToLegacySnapshot handles it correctly.
		dyn := dynSnapshots[assetEntry.ID]
			entry := shared.AssetTelemetryOverview{
				AssetID:    assetEntry.ID,
				Name:       assetEntry.Name,
				Type:       assetEntry.Type,
				Source:     assetEntry.Source,
				GroupID:    assetEntry.GroupID,
				Status:     assetEntry.Status,
				Platform:   assetEntry.Platform,
				LastSeenAt: assetEntry.LastSeenAt,
				Metrics:    dyn.ToLegacySnapshot(),
			}
			if len(dyn.Metrics) > 0 {
				entry.DynamicMetrics = dyn.Metrics
			}
			out = append(out, entry)
		}
	} else if batchStore, ok := d.TelemetryStore.(persistence.TelemetrySnapshotBatchStore); ok {
		assetIDs := make([]string, 0, len(assetList))
		for _, assetEntry := range assetList {
			assetIDs = append(assetIDs, assetEntry.ID)
		}

		snapshots, err := batchStore.SnapshotMany(assetIDs, now)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to query telemetry")
			return
		}

		for _, assetEntry := range assetList {
			metrics := snapshots[assetEntry.ID]
			out = append(out, shared.AssetTelemetryOverview{
				AssetID:    assetEntry.ID,
				Name:       assetEntry.Name,
				Type:       assetEntry.Type,
				Source:     assetEntry.Source,
				GroupID:    assetEntry.GroupID,
				Status:     assetEntry.Status,
				Platform:   assetEntry.Platform,
				LastSeenAt: assetEntry.LastSeenAt,
				Metrics:    metrics,
			})
		}
	} else {
		for _, assetEntry := range assetList {
			metrics, err := d.TelemetryStore.Snapshot(assetEntry.ID, now)
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to query telemetry")
				return
			}

			out = append(out, shared.AssetTelemetryOverview{
				AssetID:    assetEntry.ID,
				Name:       assetEntry.Name,
				Type:       assetEntry.Type,
				Source:     assetEntry.Source,
				GroupID:    assetEntry.GroupID,
				Status:     assetEntry.Status,
				Platform:   assetEntry.Platform,
				LastSeenAt: assetEntry.LastSeenAt,
				Metrics:    metrics,
			})
		}
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"generated_at": now,
		"window":       window.String(),
		"assets":       out,
	})
}

func (d *Deps) HandleAssetMetrics(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/metrics/assets/")
	if path == r.URL.Path || path == "" || strings.Contains(path, "/") {
		servicehttp.WriteError(w, http.StatusNotFound, "asset metrics path not found")
		return
	}
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	assetID := strings.TrimSpace(path)
	if d.AssetStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "asset store unavailable")
		return
	}
	if d.TelemetryStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "telemetry store unavailable")
		return
	}
	assetEntry, ok, err := d.AssetStore.GetAsset(assetID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load asset")
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "asset not found")
		return
	}

	window := parseDurationParam(r.URL.Query().Get("window"), time.Hour, 5*time.Minute, 24*time.Hour)
	step := parseDurationParam(r.URL.Query().Get("step"), shared.DefaultStepForWindow(window), 10*time.Second, 30*time.Minute)
	now := time.Now().UTC()
	start := now.Add(-window)

	series, err := d.TelemetryStore.Series(assetID, start, now, step)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to query telemetry")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"asset": map[string]any{
			"id":           assetEntry.ID,
			"name":         assetEntry.Name,
			"type":         assetEntry.Type,
			"source":       assetEntry.Source,
			"group_id":     assetEntry.GroupID,
			"status":       assetEntry.Status,
			"platform":     assetEntry.Platform,
			"last_seen_at": assetEntry.LastSeenAt,
		},
		"window": window.String(),
		"step":   step.String(),
		"from":   start,
		"to":     now,
		"series": series,
	})
}
