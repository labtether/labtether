package logspkg

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/logs"
)

func savedViewAllowed(ctx context.Context, assetID string) bool {
	if len(apiv2.AllowedAssetsFromContext(ctx)) == 0 {
		return true
	}
	assetID = strings.TrimSpace(assetID)
	return assetID != "" && apiv2.AssetCheckContext(ctx, assetID)
}

func requireSavedViewAccess(w http.ResponseWriter, r *http.Request, assetID string) bool {
	if savedViewAllowed(r.Context(), assetID) {
		return true
	}
	apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "asset-restricted api keys may only access log views for explicitly allowed assets")
	return false
}

func (d *Deps) queryAuthorizedEvents(r *http.Request, query logs.QueryRequest) ([]logs.Event, error) {
	allowed := apiv2.AllowedAssetsFromContext(r.Context())
	if len(allowed) == 0 || strings.TrimSpace(query.AssetID) != "" {
		return d.LogStore.QueryEvents(query)
	}

	targets := allowed
	if query.GroupID != "" {
		targets = query.GroupAssetIDs
	}
	if len(targets) == 0 {
		return []logs.Event{}, nil
	}

	events := make([]logs.Event, 0, query.Limit)
	for _, assetID := range targets {
		assetQuery := query
		assetQuery.AssetID = assetID
		assetQuery.GroupID = ""
		assetQuery.GroupAssetIDs = nil
		listed, err := d.LogStore.QueryEvents(assetQuery)
		if err != nil {
			return nil, err
		}
		for _, event := range listed {
			if apiv2.AssetCheck(allowed, event.AssetID) {
				events = append(events, event)
			}
		}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Timestamp.After(events[j].Timestamp) })
	if query.Limit > 0 && len(events) > query.Limit {
		events = events[:query.Limit]
	}
	return events, nil
}

func (d *Deps) authorizedSourceSummaries(r *http.Request, groupID string, includeAll bool, limit int) ([]logs.SourceSummary, time.Time, time.Time, error) {
	to := time.Now().UTC()
	from := time.Time{}
	if !includeAll {
		window := time.Hour * 24
		to = time.Now().UTC()
		from = to.Add(-window)
	}

	allowed := append([]string(nil), apiv2.AllowedAssetsFromContext(r.Context())...)
	if groupID != "" {
		_, groupAssetIDs, err := d.groupAssets(groupID)
		if err != nil {
			return nil, from, to, err
		}
		intersected := make([]string, 0, len(groupAssetIDs))
		for _, assetID := range groupAssetIDs {
			if apiv2.AssetCheck(allowed, assetID) {
				intersected = append(intersected, assetID)
			}
		}
		allowed = intersected
	}

	type aggregate struct {
		count    int
		lastSeen time.Time
	}
	stats := make(map[string]aggregate, 24)
	for _, assetID := range allowed {
		events, err := d.LogStore.QueryEvents(logs.QueryRequest{AssetID: assetID, From: from, To: to, Limit: 1000})
		if err != nil {
			return nil, from, to, err
		}
		for _, event := range events {
			if !apiv2.AssetCheck(allowed, event.AssetID) {
				continue
			}
			current := stats[event.Source]
			current.count++
			if event.Timestamp.After(current.lastSeen) {
				current.lastSeen = event.Timestamp
			}
			stats[event.Source] = current
		}
	}

	sources := make([]logs.SourceSummary, 0, len(stats))
	for source, stat := range stats {
		sources = append(sources, logs.SourceSummary{Source: source, Count: stat.count, LastSeenAt: stat.lastSeen.UTC()})
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].LastSeenAt.After(sources[j].LastSeenAt) })
	if limit > 0 && len(sources) > limit {
		sources = sources[:limit]
	}
	return sources, from, to, nil
}
