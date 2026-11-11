package persistence

// schemaMigrationChannelAllAlerts stores whether a notification channel gets
// every alert ("Send every alert here"). Before, the Hub sent alerts only to
// channels an alert route names, and the console makes no routes, so channels
// made in Settings › Notifications never got an alert.
//
// It is turned on only for a channel that no route names and that the audit
// log shows a signed-in person (a user, or the owner) created. Channels an API
// key made, channels whose creation row is gone (audit retention), routed
// channels and push (APNs) channels stay off, so none gets alerts it was never
// meant to get.
func schemaMigrationChannelAllAlerts() schemaMigration {
	return schemaMigration{Version: 112, Name: "notification_channel_all_alerts", Statements: []string{
		`ALTER TABLE notification_channels ADD COLUMN IF NOT EXISTS all_alerts BOOLEAN NOT NULL DEFAULT FALSE`,
		`UPDATE notification_channels c SET all_alerts = TRUE
		 WHERE c.type <> 'apns'
		   AND NOT EXISTS (
		     SELECT 1 FROM alert_routes r
		     WHERE jsonb_typeof(r.channel_ids) = 'array' AND r.channel_ids ? c.id)
		   AND EXISTS (
		     SELECT 1 FROM audit_events a
		     WHERE a.type = 'notification.channel.created' AND a.target = c.id
		       AND (a.actor_id = 'owner' OR a.actor_id IN (SELECT u.id FROM users u)))`,
	}}
}
