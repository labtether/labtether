package alerting

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

type incidentTimelineCursor struct {
	Version int `json:"v"`
	persistence.IncidentEventPageCursor
}

func decodeIncidentTimelineCursor(raw, incidentID string) (persistence.IncidentEventPageCursor, error) {
	if raw == "" {
		return persistence.IncidentEventPageCursor{}, nil
	}
	if len(raw) > 4096 {
		return persistence.IncidentEventPageCursor{}, persistence.ErrInvalidIncidentEventCursor
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return persistence.IncidentEventPageCursor{}, persistence.ErrInvalidIncidentEventCursor
	}
	var cursor incidentTimelineCursor
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&cursor); err != nil || cursor.Version != 1 || cursor.BeforeID == "" {
		return persistence.IncidentEventPageCursor{}, persistence.ErrInvalidIncidentEventCursor
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return persistence.IncidentEventPageCursor{}, persistence.ErrInvalidIncidentEventCursor
	}
	if err = persistence.ValidateIncidentEventCursor(incidentID, cursor.IncidentEventPageCursor); err != nil {
		return persistence.IncidentEventPageCursor{}, err
	}
	return cursor.IncidentEventPageCursor, nil
}

// HandleIncidentActions performs current incident lookup and access checks
// before every page, including requests carrying a previously issued cursor.
func (d *Deps) handleIncidentTimeline(w http.ResponseWriter, r *http.Request, incidentID string) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.IncidentEventStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "incident event store unavailable")
		return
	}
	cursor, err := decodeIncidentTimelineCursor(r.URL.Query().Get("cursor"), incidentID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid incident timeline cursor")
		return
	}
	limit := min(parseLimit(r, persistence.IncidentEventPageDefault), persistence.IncidentEventPageMaximum)
	var events []incidents.IncidentEvent
	nextCursor := ""
	if pager, ok := d.IncidentEventStore.(persistence.IncidentEventPageStore); ok {
		var page persistence.IncidentEventPage
		page, err = pager.ListIncidentEventPage(r.Context(), incidentID, cursor, limit)
		events = page.Events
		if err == nil && page.Next != nil {
			var payload []byte
			payload, err = json.Marshal(incidentTimelineCursor{Version: 1, IncidentEventPageCursor: *page.Next})
			nextCursor = base64.RawURLEncoding.EncodeToString(payload)
		}
	} else if cursor.BeforeID != "" {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "incident timeline paging unavailable")
		return
	} else {
		// Preserve the existing first-page contract for external store adapters.
		events, err = d.IncidentEventStore.ListIncidentEvents(incidentID, limit)
	}
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, persistence.ErrInvalidIncidentEventCursor) {
			code = http.StatusBadRequest
		}
		servicehttp.WriteError(w, code, "failed to list incident events")
		return
	}
	if events == nil {
		events = []incidents.IncidentEvent{}
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"events": events, "next_cursor": nextCursor})
}
