package alerting

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
	"strings"
	"time"
)

func buildIncidentLiveActivityPush(
	incident incidents.Incident,
	event string,
	registration persistence.LiveActivityPushToken,
	plaintextToken string,
	canMutate bool,
	now time.Time,
) notifications.LiveActivityPush {
	status := incidents.NormalizeStatus(incident.Status)
	if status == "" {
		status = incidents.StatusOpen
	}
	severity := incidents.NormalizeSeverity(incident.Severity)
	if severity == "" {
		severity = incidents.SeverityMedium
	}
	title := boundedIncidentPushText(incident.Title, incidentPushTitleMaxBytes)
	summary := boundedIncidentPushText(incident.Summary, 512)
	assignee := boundedIncidentPushText(incident.Assignee, 96)
	if !registration.ShowFullDetails {
		title = "Incident in progress"
		summary = ""
		assignee = ""
	}
	if title == "" {
		title = "LabTether Incident"
	}
	startedAt := incident.OpenedAt
	if startedAt.IsZero() {
		startedAt = incident.CreatedAt
	}
	if startedAt.IsZero() {
		startedAt = now
	}
	updatedAt := incident.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = now
	}
	pushEvent := "update"
	// Resolved incidents remain updateable because LabTether explicitly supports
	// reopening them. Only closed/hard-deleted incidents end the Activity; an
	// ended ActivityKit token cannot be restarted remotely without push-to-start.
	if status == incidents.StatusClosed {
		pushEvent = "end"
	}
	staleAt := now.Add(20 * time.Minute)
	dismissAt := now
	push := notifications.LiveActivityPush{
		DeviceToken: plaintextToken,
		BundleID:    registration.BundleID,
		ActivityID:  registration.ActivityID,
		Event:       pushEvent,
		Timestamp:   now,
		ExpiresAt:   registration.ExpiresAt,
		Priority:    liveActivityPushPriority(event, pushEvent, severity),
		State: notifications.LiveActivityContentState{
			Title:           title,
			Summary:         summary,
			Status:          status,
			Severity:        severity,
			Assignee:        assignee,
			StartedAt:       startedAt,
			UpdatedAt:       updatedAt,
			ShowFullDetails: registration.ShowFullDetails,
			CanMutate:       canMutate,
		},
	}
	if pushEvent == "end" {
		push.DismissAt = &dismissAt
	} else {
		push.StaleAt = &staleAt
	}
	return push
}

func liveActivityPushPriority(event, pushEvent, severity string) int {
	if pushEvent == "end" || severity == incidents.SeverityCritical {
		return 10
	}
	switch strings.ToLower(strings.TrimSpace(event)) {
	case "incident.updated", "incident.registered":
		return 5
	default:
		return 10
	}
}

func liveActivityTokenHashMatches(token, expectedHash string) bool {
	digest := sha256.Sum256([]byte(token))
	actual := hex.EncodeToString(digest[:])
	return len(actual) == len(expectedHash) && subtle.ConstantTimeCompare([]byte(actual), []byte(expectedHash)) == 1
}

func liveActivityChannelKey(bundleID, environment string) string {
	return strings.TrimSpace(bundleID) + "\x00" + strings.ToLower(strings.TrimSpace(environment))
}

func incidentLiveActivityContentChanged(previous, current incidents.Incident) bool {
	return previous.Title != current.Title ||
		previous.Summary != current.Summary ||
		previous.Status != current.Status ||
		previous.Severity != current.Severity ||
		previous.Assignee != current.Assignee ||
		!previous.OpenedAt.Equal(current.OpenedAt)
}

type pendingLiveActivityIncidentState struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary,omitempty"`
	Status    string    `json:"status"`
	Severity  string    `json:"severity"`
	Assignee  string    `json:"assignee,omitempty"`
	OpenedAt  time.Time `json:"opened_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (d *Deps) encryptPendingLiveActivityIncident(recordID string, incident incidents.Incident) (string, error) {
	if d.NotificationSecrets == nil {
		return "", errNotificationSecretsUnavailable
	}
	payload, err := json.Marshal(pendingLiveActivityIncidentState{
		ID: incident.ID, Title: incident.Title, Summary: incident.Summary,
		Status: incident.Status, Severity: incident.Severity, Assignee: incident.Assignee,
		OpenedAt: incident.OpenedAt, CreatedAt: incident.CreatedAt, UpdatedAt: incident.UpdatedAt,
	})
	if err != nil {
		return "", err
	}
	return d.NotificationSecrets.EncryptString(string(payload), pendingLiveActivityStateAAD(recordID))
}

func (d *Deps) decryptPendingLiveActivityIncident(registration persistence.LiveActivityPushToken) (incidents.Incident, bool) {
	if d.NotificationSecrets == nil || strings.TrimSpace(registration.PendingStateCiphertext) == "" {
		return incidents.Incident{}, false
	}
	plaintext, err := d.NotificationSecrets.DecryptString(
		registration.PendingStateCiphertext,
		pendingLiveActivityStateAAD(registration.ID),
	)
	if err != nil {
		return incidents.Incident{}, false
	}
	var state pendingLiveActivityIncidentState
	if err := json.Unmarshal([]byte(plaintext), &state); err != nil || strings.TrimSpace(state.ID) == "" {
		return incidents.Incident{}, false
	}
	return incidents.Incident{
		ID: state.ID, Title: state.Title, Summary: state.Summary,
		Status: state.Status, Severity: state.Severity, Assignee: state.Assignee,
		OpenedAt: state.OpenedAt, CreatedAt: state.CreatedAt, UpdatedAt: state.UpdatedAt,
	}, true
}

func pendingLiveActivityStateAAD(recordID string) string {
	return "live-activity-pending-state:" + recordID
}
