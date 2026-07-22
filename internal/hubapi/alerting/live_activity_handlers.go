package alerting

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

const liveActivityRegistrationLifetime = 12 * time.Hour

type registerLiveActivityRequest struct {
	DeviceID        string `json:"device_id"`
	PushToken       string `json:"push_token"`
	BundleID        string `json:"bundle_id"`
	Environment     string `json:"environment"`
	ShowFullDetails bool   `json:"show_full_details"`
}

// HandleIncidentLiveActivityTokens owns the authenticated registration
// contract at /live-activities/incidents/{incident}/activities/{activity}.
// Tokens are never returned by this endpoint and reach persistence only after
// encryption with row-bound AAD.
func (d *Deps) HandleIncidentLiveActivityTokens(w http.ResponseWriter, r *http.Request) {
	if d == nil || d.LiveActivityStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "live activity store unavailable")
		return
	}
	userID := strings.TrimSpace(apiv2.UserIDFromContext(r.Context()))
	if userID == "" {
		servicehttp.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	// A long-lived ActivityKit binding cannot safely retain an API key's
	// per-token asset allowlist after the request context disappears. The mobile
	// app uses interactive/OIDC user sessions, whose current user role is checked
	// again at delivery time. Reject scoped API-key principals fail closed.
	if strings.TrimSpace(apiv2.APIKeyIDFromContext(r.Context())) != "" {
		servicehttp.WriteError(w, http.StatusForbidden, "live activities require an interactive user session")
		return
	}
	incidentID, activityID, ok := parseLiveActivityTokenPath(r.URL.Path)
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "live activity path not found")
		return
	}
	if !boundedLiveActivityIdentifier(activityID, 128) {
		servicehttp.WriteError(w, http.StatusBadRequest, "activity id is invalid")
		return
	}
	if r.Method == http.MethodDelete {
		if d.EnforceRateLimit != nil && !d.EnforceRateLimit(w, r, "live_activities.deregister", 120, time.Minute) {
			return
		}
		deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
		if !boundedLiveActivityIdentifier(deviceID, 128) {
			servicehttp.WriteError(w, http.StatusBadRequest, "device_id is required and must be valid")
			return
		}
		registrationID := strings.TrimSpace(r.URL.Query().Get("registration_id"))
		if registrationID != "" && !boundedLiveActivityIdentifier(registrationID, 128) {
			servicehttp.WriteError(w, http.StatusBadRequest, "registration_id is invalid")
			return
		}
		var err error
		if registrationID != "" {
			err = d.LiveActivityStore.DeleteLiveActivityPushTokenByOwnerAndID(
				r.Context(), userID, deviceID, activityID, incidentID, registrationID,
			)
		} else {
			err = d.LiveActivityStore.DeleteLiveActivityPushToken(
				r.Context(), userID, deviceID, activityID, incidentID,
			)
		}
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to deregister live activity")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.NotificationSecrets == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "live activity token encryption unavailable")
		return
	}
	if d.IncidentStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "incident store unavailable")
		return
	}
	incident, found, err := d.IncidentStore.GetIncident(incidentID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load incident")
		return
	}
	if !found {
		servicehttp.WriteError(w, http.StatusNotFound, "incident not found")
		return
	}
	if !d.requireIncidentAccess(w, r, incident) {
		return
	}

	switch r.Method {
	case http.MethodPut, http.MethodPost:
		if d.EnforceRateLimit != nil && !d.EnforceRateLimit(w, r, "live_activities.register", 120, time.Minute) {
			return
		}
		var request registerLiveActivityRequest
		if err := decodeJSONBody(w, r, &request); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid live activity payload")
			return
		}
		normalizeLiveActivityRegistration(&request)
		if message := validateLiveActivityRegistration(request); message != "" {
			servicehttp.WriteError(w, http.StatusBadRequest, message)
			return
		}

		recordID := idgen.New("lat")
		ciphertext, encryptErr := d.NotificationSecrets.EncryptString(
			request.PushToken,
			liveActivityTokenAAD(recordID),
		)
		if encryptErr != nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "live activity token encryption unavailable")
			return
		}
		hash := sha256.Sum256([]byte(request.PushToken))
		expiresAt := time.Now().UTC().Add(liveActivityRegistrationLifetime)
		if err := d.LiveActivityStore.UpsertLiveActivityPushToken(r.Context(), persistence.LiveActivityPushToken{
			ID:              recordID,
			UserID:          userID,
			DeviceID:        request.DeviceID,
			ActivityID:      activityID,
			IncidentID:      incidentID,
			TokenCiphertext: ciphertext,
			TokenHash:       hex.EncodeToString(hash[:]),
			BundleID:        request.BundleID,
			Environment:     request.Environment,
			ShowFullDetails: request.ShowFullDetails,
			ExpiresAt:       expiresAt,
		}); errors.Is(err, persistence.ErrLiveActivityRegistrationLimit) {
			servicehttp.WriteError(w, http.StatusTooManyRequests, "live activity registration limit reached")
			return
		} else if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to register live activity")
			return
		}
		// Token issuance is asynchronous relative to incident transitions. Queue
		// the state loaded under this authenticated request immediately after the
		// binding commits so a transition that happened before registration is not
		// permanently missed (including a terminal close/delete reconciliation).
		if d.DispatchIncidentLiveActivity != nil {
			d.DispatchIncidentLiveActivity(incident, "incident.registered")
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"registered":      true,
			"registration_id": recordID,
			"activity_id":     activityID,
			"incident_id":     incidentID,
			"expires_at":      expiresAt,
		})

	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func parseLiveActivityTokenPath(path string) (incidentID, activityID string, ok bool) {
	path = strings.TrimPrefix(path, "/live-activities/incidents/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 || parts[1] != "activities" {
		return "", "", false
	}
	incidentID = strings.TrimSpace(parts[0])
	activityID = strings.TrimSpace(parts[2])
	if !boundedLiveActivityIdentifier(incidentID, 255) || activityID == "" {
		return "", "", false
	}
	return incidentID, activityID, true
}

func normalizeLiveActivityRegistration(request *registerLiveActivityRequest) {
	request.DeviceID = strings.TrimSpace(request.DeviceID)
	request.PushToken = strings.ToLower(strings.TrimSpace(request.PushToken))
	request.BundleID = strings.TrimSpace(request.BundleID)
	request.Environment = strings.ToLower(strings.TrimSpace(request.Environment))
}

func validateLiveActivityRegistration(request registerLiveActivityRequest) string {
	if !boundedLiveActivityIdentifier(request.DeviceID, 128) {
		return "device_id is required and must be valid"
	}
	if !notifications.ValidLiveActivityPushToken(request.PushToken) {
		return "push_token must be a bounded hexadecimal ActivityKit token"
	}
	if !validLiveActivityBundleID(request.BundleID) {
		return "bundle_id must be a valid application bundle identifier"
	}
	if request.Environment != "sandbox" && request.Environment != "production" {
		return "environment must be sandbox or production"
	}
	return ""
}

func boundedLiveActivityIdentifier(value string, maxRunes int) bool {
	value = strings.TrimSpace(value)
	if value == "" || len([]rune(value)) > maxRunes {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) || character == '/' || character == '\\' {
			return false
		}
	}
	return true
}

func validLiveActivityBundleID(value string) bool {
	if value == "" || len(value) > 255 || strings.HasSuffix(value, liveActivityTopicSuffixForValidation) {
		return false
	}
	if !strings.Contains(value, ".") || strings.HasPrefix(value, ".") || strings.HasSuffix(value, ".") {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '.' || character == '-' {
			continue
		}
		return false
	}
	return !strings.Contains(value, "..")
}

const liveActivityTopicSuffixForValidation = ".push-type.liveactivity"

func liveActivityTokenAAD(recordID string) string {
	return "live-activity-push-token:" + recordID
}
