package persistence

import (
	"encoding/json"
	"strings"

	"github.com/labtether/labtether/internal/audit"
)

// auditTargetNameKey stores an event's TargetName inside its details JSON,
// so the name needs no new column (2.0 keeps the 1.7.1 schema). Reads move
// it back to TargetName and out of Details.
const auditTargetNameKey = "target_name"

// auditDetailsWithTargetNameSQL is the details value an audit insert stores
// ($2 type, $4 target, $9 details). When the writer gave no name and the
// target is an object that still exists, the insert adds that object's name
// itself, so every writer gets it without its own lookup. Saved actions are
// left out: their names stay private to the person who saved them. Request
// rows target a method and path, never an object.
const auditDetailsWithTargetNameSQL = `CASE
	WHEN $9::jsonb ? 'target_name' OR $2::text = 'api.request' OR COALESCE($4::text, '') = '' THEN $9::jsonb
	ELSE jsonb_strip_nulls(jsonb_build_object('target_name', COALESCE(
		(SELECT name FROM assets WHERE id = $4::text),
		(SELECT name FROM webhooks WHERE id = $4::text),
		(SELECT name FROM scheduled_tasks WHERE id = $4::text),
		(SELECT username FROM users WHERE id = $4::text),
		(SELECT name FROM api_keys WHERE id = $4::text),
		(SELECT name FROM notification_channels WHERE id = $4::text),
		(SELECT name FROM alert_rules WHERE id = $4::text),
		(SELECT name FROM groups WHERE id = $4::text),
		(SELECT name FROM update_plans WHERE id = $4::text),
		(SELECT name FROM credential_profiles WHERE id = $4::text),
		(SELECT NULLIF(label, '') FROM enrollment_tokens WHERE id = $4::text)
	))) || $9::jsonb
END`

func marshalAuditDetails(event audit.Event) ([]byte, error) {
	name := strings.TrimSpace(event.TargetName)
	if event.Details == nil && name == "" {
		return []byte("{}"), nil
	}
	details := make(map[string]any, len(event.Details)+1)
	for key, value := range event.Details {
		details[key] = value
	}
	if name != "" {
		details[auditTargetNameKey] = name
	}
	return json.Marshal(details)
}

func unmarshalAuditDetails(event *audit.Event, payload []byte) {
	if len(payload) == 0 {
		return
	}
	mapped := map[string]any{}
	if err := json.Unmarshal(payload, &mapped); err != nil {
		return
	}
	if name, ok := mapped[auditTargetNameKey].(string); ok {
		event.TargetName = name
		delete(mapped, auditTargetNameKey)
	}
	event.Details = mapped
}
