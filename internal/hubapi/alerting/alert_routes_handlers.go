package alerting

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) HandleAlertRoutes(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/alerts/routes" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.NotificationStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "notification store unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		routes, err := d.NotificationStore.ListAlertRoutes(parseLimit(r, 50))
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list alert routes")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"routes": routes})
	case http.MethodPost:
		if !d.EnforceRateLimit(w, r, "alerts.routes.create", 60, time.Minute) {
			return
		}
		var req notifications.CreateRouteRequest
		if err := decodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid route payload")
			return
		}
		if err := ValidateCreateRouteRequest(req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		route, err := d.NotificationStore.CreateAlertRoute(req)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create alert route")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"route": route})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) HandleAlertRouteActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/alerts/routes/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "route path not found")
		return
	}
	if d.NotificationStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "notification store unavailable")
		return
	}

	routeID := strings.TrimSpace(path)
	switch r.Method {
	case http.MethodGet:
		route, ok, err := d.NotificationStore.GetAlertRoute(routeID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load route")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "route not found")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"route": route})
	case http.MethodPatch, http.MethodPut:
		if !d.EnforceRateLimit(w, r, "alerts.routes.update", 120, time.Minute) {
			return
		}
		var req notifications.UpdateRouteRequest
		if err := decodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid route payload")
			return
		}
		if err := ValidateUpdateRouteRequest(req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		updated, err := d.NotificationStore.UpdateAlertRoute(routeID, req)
		if err != nil {
			if err == notifications.ErrRouteNotFound {
				servicehttp.WriteError(w, http.StatusNotFound, "route not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update route")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"route": updated})
	case http.MethodDelete:
		if err := d.NotificationStore.DeleteAlertRoute(routeID); err != nil {
			if err == notifications.ErrRouteNotFound {
				servicehttp.WriteError(w, http.StatusNotFound, "route not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete route")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
