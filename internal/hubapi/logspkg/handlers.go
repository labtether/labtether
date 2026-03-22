package logspkg

import (
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

// HandleLogsQuery handles GET /logs/query.
func (d *Deps) HandleLogsQuery(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/logs/query" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	window := shared.ParseDurationParam(r.URL.Query().Get("window"), time.Hour, time.Minute, 7*24*time.Hour)
	to := shared.ParseTimestampParam(r.URL.Query().Get("to"), time.Now().UTC())
	from := shared.ParseTimestampParam(r.URL.Query().Get("from"), to.Add(-window))
	requestedLimit := shared.ParseLimit(r, 200)
	groupID := shared.GroupIDQueryParam(r)
	includeHeartbeats := ParseIncludeHeartbeatsQuery(r.URL.Query().Get("include_heartbeats"))
	includeFields := ParseIncludeFieldsQuery(r.URL.Query().Get("include_fields"))
	assetGroup := map[string]string{}
	groupAssetIDs := []string(nil)

	queryLimit := requestedLimit
	if !includeHeartbeats {
		queryLimit = requestedLimit * 5
		if queryLimit < 200 {
			queryLimit = 200
		}
		if queryLimit > 1000 {
			queryLimit = 1000
		}
	}
	if groupID != "" {
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
		groupQueryLimit := requestedLimit * 4
		if groupQueryLimit < 200 {
			groupQueryLimit = 200
		}
		if groupQueryLimit > 1000 {
			groupQueryLimit = 1000
		}
		if groupQueryLimit > queryLimit {
			queryLimit = groupQueryLimit
		}

		assetList, err := d.AssetStore.ListAssets()
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list assets")
			return
		}
		assetGroup = make(map[string]string, len(assetList))
		for _, assetEntry := range assetList {
			if strings.TrimSpace(assetEntry.GroupID) != "" {
				assetGroup[assetEntry.ID] = strings.TrimSpace(assetEntry.GroupID)
			}
		}
		groupAssetIDs = shared.GroupAssetIDsForGroup(groupID, assetGroup)
	}

	query := logs.QueryRequest{
		AssetID:       strings.TrimSpace(r.URL.Query().Get("asset_id")),
		Source:        strings.TrimSpace(r.URL.Query().Get("source")),
		Level:         strings.TrimSpace(r.URL.Query().Get("level")),
		Search:        strings.TrimSpace(r.URL.Query().Get("q")),
		GroupID:       groupID,
		GroupAssetIDs: groupAssetIDs,
		From:          from,
		To:            to,
		Limit:         queryLimit,
		ExcludeFields: !includeFields && groupID == "",
	}
	if groupID != "" && !includeFields {
		query.FieldKeys = []string{"group_id"}
	}

	events, err := d.LogStore.QueryEvents(query)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to query logs")
		return
	}

	if groupID != "" {
		events = shared.FilterLogEventsByGroup(events, groupID, assetGroup)
	}

	if !includeHeartbeats {
		events = FilterHeartbeatNoise(events)
	}

	if len(events) > requestedLimit {
		events = events[:requestedLimit]
	}
	if !includeFields {
		for i := range events {
			events[i].Fields = nil
		}
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"from":   from,
		"to":     to,
		"window": window.String(),
		"events": events,
	})
}

// ParseIncludeHeartbeatsQuery parses the include_heartbeats query parameter.
// The default (empty string) is true.
func ParseIncludeHeartbeatsQuery(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// ParseIncludeFieldsQuery parses the include_fields query parameter.
// The default (empty string) is true.
func ParseIncludeFieldsQuery(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// FilterHeartbeatNoise removes heartbeat noise events from the slice.
func FilterHeartbeatNoise(events []logs.Event) []logs.Event {
	filtered := make([]logs.Event, 0, len(events))
	for _, event := range events {
		if isHeartbeatNoiseEvent(event) {
			continue
		}
		filtered = append(filtered, event)
	}
	return filtered
}

func isHeartbeatNoiseEvent(event logs.Event) bool {
	message := strings.ToLower(strings.TrimSpace(event.Message))
	return strings.HasPrefix(message, "heartbeat received (")
}

// HandleLogSources handles GET /logs/sources.
func (d *Deps) HandleLogSources(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/logs/sources" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	startedAt := time.Now().UTC()
	caller := shared.SourceQueryCaller(r, "api.logs.sources")
	limit := shared.ParseLimit(r, 50)
	groupID := shared.GroupIDQueryParam(r)
	includeAll := ParseIncludeAllSourcesQuery(r.URL.Query().Get("all"))
	groupFiltered := groupID != ""
	mode := "unknown"
	cacheHit := false
	traceWindowStart := time.Unix(0, 0).UTC()
	traceWindowEnd := time.Unix(0, 0).UTC()
	var traceErr error

	var sources []logs.SourceSummary
	defer func() {
		shared.LogSourceQueryDiagnostic(
			"/logs/sources",
			caller,
			mode,
			groupFiltered,
			cacheHit,
			traceWindowStart,
			traceWindowEnd,
			limit,
			len(sources),
			startedAt,
			traceErr,
		)
	}()

	if groupID == "" {
		if includeAll {
			mode = "all_time"
			listed, err := d.LogStore.ListSources(limit)
			if err != nil {
				traceErr = err
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list log sources")
				return
			}
			sources = listed
		} else if recentLister, ok := d.LogStore.(RecentSourceLister); ok && d.ListRecentSourcesCached != nil {
			window := shared.ParseDurationParam(r.URL.Query().Get("window"), 24*time.Hour, time.Minute, 30*24*time.Hour)
			toRaw := strings.TrimSpace(r.URL.Query().Get("to"))
			fromRaw := strings.TrimSpace(r.URL.Query().Get("from"))
			to := shared.CeilToMinute(shared.ParseTimestampParam(toRaw, time.Now().UTC()))
			from := shared.ParseTimestampParam(fromRaw, to.Add(-window))
			if fromRaw == "" {
				from = to.Add(-window)
			}
			from = shared.FloorToMinute(from)
			if !from.Before(to) {
				from = shared.FloorToMinute(to.Add(-window))
			}
			traceWindowStart = from
			traceWindowEnd = to
			mode = "recent_window"
			listed, hit, err := d.ListRecentSourcesCached(recentLister, limit, from)
			if err != nil {
				traceErr = err
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list log sources")
				return
			}
			cacheHit = hit
			if hit {
				mode = "recent_window_cache"
			}
			sources = listed
		} else {
			mode = "all_time_fallback"
			listed, err := d.LogStore.ListSources(limit)
			if err != nil {
				traceErr = err
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list log sources")
				return
			}
			sources = listed
		}
	} else {
		if d.GroupStore == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "group store unavailable")
			return
		}
		_, ok, err := d.GroupStore.GetGroup(groupID)
		if err != nil {
			traceErr = err
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load group")
			return
		}
		if !ok {
			traceErr = errors.New("group not found")
			servicehttp.WriteError(w, http.StatusNotFound, "group not found")
			return
		}

		mode = "group_filtered_window"
		window := shared.ParseDurationParam(r.URL.Query().Get("window"), 24*time.Hour, time.Minute, 30*24*time.Hour)
		to := shared.CeilToMinute(shared.ParseTimestampParam(r.URL.Query().Get("to"), time.Now().UTC()))
		from := shared.ParseTimestampParam(r.URL.Query().Get("from"), to.Add(-window))
		from = shared.FloorToMinute(from)
		if !from.Before(to) {
			from = shared.FloorToMinute(to.Add(-window))
		}
		traceWindowStart = from
		traceWindowEnd = to

		assetList, err := d.AssetStore.ListAssets()
		if err != nil {
			traceErr = err
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list assets")
			return
		}
		assetGroup := make(map[string]string, len(assetList))
		for _, assetEntry := range assetList {
			if strings.TrimSpace(assetEntry.GroupID) != "" {
				assetGroup[assetEntry.ID] = strings.TrimSpace(assetEntry.GroupID)
			}
		}
		groupAssetIDs := shared.GroupAssetIDsForGroup(groupID, assetGroup)

		events, err := d.LogStore.QueryEvents(logs.QueryRequest{
			From:          from,
			To:            to,
			Limit:         1000,
			GroupID:       groupID,
			GroupAssetIDs: groupAssetIDs,
			FieldKeys:     []string{"group_id"},
		})
		if err != nil {
			traceErr = err
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to query logs for source aggregation")
			return
		}
		events = shared.FilterLogEventsByGroup(events, groupID, assetGroup)

		type aggregate struct {
			count    int
			lastSeen time.Time
		}
		stats := make(map[string]aggregate, 24)
		for _, event := range events {
			current := stats[event.Source]
			current.count++
			if event.Timestamp.After(current.lastSeen) {
				current.lastSeen = event.Timestamp
			}
			stats[event.Source] = current
		}

		sources = make([]logs.SourceSummary, 0, len(stats))
		for source, stat := range stats {
			sources = append(sources, logs.SourceSummary{
				Source:     source,
				Count:      stat.count,
				LastSeenAt: stat.lastSeen.UTC(),
			})
		}
		sort.Slice(sources, func(i, j int) bool {
			return sources[i].LastSeenAt.After(sources[j].LastSeenAt)
		})
		if len(sources) > limit {
			sources = sources[:limit]
		}
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"sources": sources,
	})
}

// ParseIncludeAllSourcesQuery parses the all query parameter.
// The default (empty string) is false.
func ParseIncludeAllSourcesQuery(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// HandleLogViews handles GET and POST /logs/views.
func (d *Deps) HandleLogViews(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/logs/views" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		views, err := d.LogStore.ListViews(shared.ParseLimit(r, 50))
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list log views")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"views": views})
	case http.MethodPost:
		var req logs.SavedViewRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid log view payload")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}

		view, err := d.LogStore.SaveView(req)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to save log view")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"view": view})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleLogViewActions handles GET, PUT/PATCH, and DELETE /logs/views/{id}.
func (d *Deps) HandleLogViewActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/logs/views/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "log view path not found")
		return
	}

	viewID := strings.TrimSpace(path)
	if strings.Contains(viewID, "/") {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown log view action")
		return
	}

	switch r.Method {
	case http.MethodGet:
		view, ok, err := d.LogStore.GetView(viewID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load log view")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "log view not found")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"view": view})
	case http.MethodPut, http.MethodPatch:
		var req logs.SavedViewRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid log view payload")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		view, err := d.LogStore.UpdateView(viewID, req)
		if err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "log view not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update log view")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"view": view})
	case http.MethodDelete:
		if err := d.LogStore.DeleteView(viewID); err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "log view not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete log view")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "view_id": viewID})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
