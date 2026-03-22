package resources

import (
	"net/http"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

type RegisterDeviceRequest struct {
	DeviceID  string `json:"device_id"`
	Platform  string `json:"platform"`
	PushToken string `json:"push_token"`
	BundleID  string `json:"bundle_id"`
}

func (d *Deps) HandleDeviceRegister(w http.ResponseWriter, r *http.Request) {
	userID := d.UserIDFromContext(r.Context())
	if userID == "" {
		servicehttp.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	switch r.Method {
	case http.MethodPost:
		var req RegisterDeviceRequest
		if err := d.DecodeJSONBody(w, r, &req); err != nil {
			return
		}
		if req.DeviceID == "" || req.PushToken == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "device_id and push_token are required")
			return
		}
		if req.Platform == "" {
			req.Platform = "ios"
		}
		err := d.DB.UpsertPushDevice(r.Context(), persistence.PushDevice{
			UserID:    userID,
			DeviceID:  req.DeviceID,
			Platform:  req.Platform,
			PushToken: req.PushToken,
			BundleID:  req.BundleID,
		})
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to register device")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"registered": true, "device_id": req.DeviceID})

	case http.MethodDelete:
		deviceID := r.URL.Query().Get("device_id")
		if deviceID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "device_id query parameter required")
			return
		}
		if err := d.DB.DeletePushDevice(r.Context(), userID, deviceID); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to deregister device")
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
