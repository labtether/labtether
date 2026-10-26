package resources

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode"

	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

// RegisterDeviceRequest carries exactly one push credential: push_token (a
// raw APNs token, for Hubs with their own APNs key) or relay_registration_id
// (from the LabTether push relay; a case-sensitive bearer secret).
type RegisterDeviceRequest struct {
	DeviceID               string `json:"device_id"`
	Platform               string `json:"platform"`
	PushToken              string `json:"push_token"`
	RelayRegistrationID    string `json:"relay_registration_id"`
	BundleID               string `json:"bundle_id"`
	Environment            string `json:"environment"`
	TimeZone               string `json:"time_zone"`
	NotifyCriticalAlerts   bool   `json:"notify_critical_alerts"`
	NotifyNodeOffline      bool   `json:"notify_node_offline"`
	NotifyServiceDown      bool   `json:"notify_service_down"`
	PushCategory           string `json:"push_category"`
	MinimumSeverity        string `json:"minimum_severity"`
	QuietHoursEnabled      bool   `json:"quiet_hours_enabled"`
	QuietHoursStartMinutes int    `json:"quiet_hours_start_minutes"`
	QuietHoursEndMinutes   int    `json:"quiet_hours_end_minutes"`
	DigestWindowSeconds    int    `json:"digest_window_seconds"`
}

type DeregisterDeviceRequest struct {
	DeviceID string `json:"device_id"`
}

func defaultRegisterDeviceRequest() RegisterDeviceRequest {
	return RegisterDeviceRequest{
		Platform:               "ios",
		NotifyCriticalAlerts:   true,
		NotifyNodeOffline:      true,
		NotifyServiceDown:      true,
		PushCategory:           "critical_only",
		MinimumSeverity:        "warning",
		QuietHoursStartMinutes: 22 * 60,
		QuietHoursEndMinutes:   7 * 60,
		DigestWindowSeconds:    180,
	}
}

func (d *Deps) HandleDeviceRegister(w http.ResponseWriter, r *http.Request) {
	userID := d.UserIDFromContext(r.Context())
	if userID == "" {
		servicehttp.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	switch r.Method {
	case http.MethodPost:
		if d.EnforceRateLimit != nil && !d.EnforceRateLimit(w, r, "push_devices.register", 60, time.Minute) {
			return
		}
		req := defaultRegisterDeviceRequest()
		if err := d.DecodeJSONBody(w, r, &req); err != nil {
			return
		}
		if errMessage := normalizeAndValidateDeviceRegistration(&req); errMessage != "" {
			servicehttp.WriteError(w, http.StatusBadRequest, errMessage)
			return
		}
		pushToken, transport := req.PushToken, notifications.APNsTransportDirect
		if req.RelayRegistrationID != "" {
			pushToken, transport = req.RelayRegistrationID, notifications.APNsTransportRelay
		}
		err := d.DB.UpsertPushDevice(r.Context(), persistence.PushDevice{
			UserID:                 userID,
			DeviceID:               req.DeviceID,
			Platform:               req.Platform,
			PushToken:              pushToken,
			PushTransport:          transport,
			BundleID:               req.BundleID,
			Environment:            req.Environment,
			TimeZone:               req.TimeZone,
			NotifyCriticalAlerts:   req.NotifyCriticalAlerts,
			NotifyNodeOffline:      req.NotifyNodeOffline,
			NotifyServiceDown:      req.NotifyServiceDown,
			PushCategory:           req.PushCategory,
			MinimumSeverity:        req.MinimumSeverity,
			QuietHoursEnabled:      req.QuietHoursEnabled,
			QuietHoursStartMinutes: req.QuietHoursStartMinutes,
			QuietHoursEndMinutes:   req.QuietHoursEndMinutes,
			DigestWindowSeconds:    req.DigestWindowSeconds,
		})
		if err != nil {
			if errors.Is(err, persistence.ErrPushDeviceRegistrationLimit) {
				servicehttp.WriteError(w, http.StatusTooManyRequests, "push device registration limit reached")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to register device")
			return
		}
		if transport == notifications.APNsTransportRelay {
			d.relayPushDeviceRegistered()
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"registered":     true,
			"device_id":      req.DeviceID,
			"push_transport": transport,
		})

	case http.MethodDelete:
		if d.EnforceRateLimit != nil && !d.EnforceRateLimit(w, r, "push_devices.deregister", 60, time.Minute) {
			return
		}
		d.deletePushDevice(w, r, userID, false)

	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleDeviceDeregister preserves compatibility with clients that used the
// original POST /api/v1/devices/deregister contract. New clients should use
// DELETE /api/v1/devices/register?device_id=... instead.
func (d *Deps) HandleDeviceDeregister(w http.ResponseWriter, r *http.Request) {
	userID := d.UserIDFromContext(r.Context())
	if userID == "" {
		servicehttp.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	switch r.Method {
	case http.MethodPost, http.MethodDelete:
		if d.EnforceRateLimit != nil && !d.EnforceRateLimit(w, r, "push_devices.deregister", 60, time.Minute) {
			return
		}
		d.deletePushDevice(w, r, userID, true)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) deletePushDevice(w http.ResponseWriter, r *http.Request, userID string, allowJSONBody bool) {
	deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
	if deviceID == "" && allowJSONBody {
		var req DeregisterDeviceRequest
		if err := d.DecodeJSONBody(w, r, &req); err != nil {
			return
		}
		deviceID = strings.TrimSpace(req.DeviceID)
	}
	if deviceID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "device_id is required")
		return
	}
	if !isBoundedPrintable(deviceID, 128) {
		servicehttp.WriteError(w, http.StatusBadRequest, "device_id must be at most 128 characters and contain no control characters")
		return
	}
	if err := d.DB.DeletePushDevice(r.Context(), userID, deviceID); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to deregister device")
		return
	}
	if r.Method == http.MethodDelete {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"registered": false, "device_id": deviceID})
}

func normalizeAndValidateDeviceRegistration(req *RegisterDeviceRequest) string {
	req.DeviceID = strings.TrimSpace(req.DeviceID)
	req.Platform = strings.ToLower(strings.TrimSpace(req.Platform))
	req.PushToken = strings.TrimSpace(req.PushToken)
	// Relay ids are case-sensitive bearer secrets: trim, never change case.
	req.RelayRegistrationID = strings.TrimSpace(req.RelayRegistrationID)
	req.BundleID = strings.TrimSpace(req.BundleID)
	req.Environment = strings.ToLower(strings.TrimSpace(req.Environment))
	req.TimeZone = strings.TrimSpace(req.TimeZone)
	req.PushCategory = strings.ToLower(strings.TrimSpace(req.PushCategory))
	req.MinimumSeverity = normalizePushSeverity(req.MinimumSeverity)

	if req.PushToken != "" && req.RelayRegistrationID != "" {
		return "send either push_token or relay_registration_id, not both"
	}
	if req.DeviceID == "" || (req.PushToken == "" && req.RelayRegistrationID == "") {
		return "device_id and push_token (or relay_registration_id) are required"
	}
	if !isBoundedPrintable(req.DeviceID, 128) {
		return "device_id must be at most 128 characters and contain no control characters"
	}
	if req.RelayRegistrationID != "" {
		if !notifications.ValidRelayRegistrationID(req.RelayRegistrationID) {
			return "relay_registration_id must be a 43-character push relay registration id"
		}
		if req.BundleID == "" {
			req.BundleID = notifications.OfficialAppBundleID
		}
		if req.BundleID != notifications.OfficialAppBundleID {
			return "relay registrations are only accepted for " + notifications.OfficialAppBundleID
		}
	} else if !isBoundedPrintable(req.PushToken, 512) {
		return "push_token must be at most 512 characters and contain no control characters"
	}
	if !isBoundedPrintable(req.BundleID, 255) {
		return "bundle_id must be at most 255 characters and contain no control characters"
	}
	if req.Platform == "" {
		req.Platform = "ios"
	}
	if req.Platform != "ios" && req.Platform != "ipados" {
		return "platform must be ios or ipados"
	}
	if req.Environment != "" && req.Environment != "sandbox" && req.Environment != "production" {
		return "environment must be sandbox or production"
	}
	if req.TimeZone != "" && !isBoundedPrintable(req.TimeZone, 64) {
		return "time_zone must be at most 64 characters and contain no control characters"
	}
	if req.TimeZone != "" {
		if _, err := time.LoadLocation(req.TimeZone); err != nil {
			return "time_zone must be a valid IANA timezone identifier"
		}
	}
	switch req.PushCategory {
	case "critical_only", "all_alerts", "alerts_and_incidents":
	default:
		return "push_category must be critical_only, all_alerts, or alerts_and_incidents"
	}
	if req.MinimumSeverity == "" {
		return "minimum_severity must be info, warning, high, or critical"
	}
	if req.QuietHoursStartMinutes < 0 || req.QuietHoursStartMinutes > 1439 {
		return "quiet_hours_start_minutes must be between 0 and 1439"
	}
	if req.QuietHoursEndMinutes < 0 || req.QuietHoursEndMinutes > 1439 {
		return "quiet_hours_end_minutes must be between 0 and 1439"
	}
	if req.DigestWindowSeconds < 30 || req.DigestWindowSeconds > 86400 {
		return "digest_window_seconds must be between 30 and 86400"
	}
	return ""
}

// relayPushDeviceRegistered lets the Hub create its default relay channel
// and route the first time a relay device registers. It runs detached from
// the request, so a client disconnect cannot stop it halfway.
func (d *Deps) relayPushDeviceRegistered() {
	if d.OnRelayPushDeviceRegistered == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	d.OnRelayPushDeviceRegistered(ctx)
}

func isBoundedPrintable(value string, maxRunes int) bool {
	if len([]rune(value)) > maxRunes {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func normalizePushSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "info", "low":
		return "info"
	case "warning", "medium":
		return "warning"
	case "high", "major":
		return "high"
	case "critical":
		return "critical"
	default:
		return ""
	}
}
