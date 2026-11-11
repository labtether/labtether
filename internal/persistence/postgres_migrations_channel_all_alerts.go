package persistence

// schemaMigrationChannelAllAlerts stores whether a notification channel gets
// every alert ("Send every alert here"). Before, the Hub sent alerts only to
// channels an alert route names, and the console makes no routes, so channels
// made in Settings › Notifications never got an alert. Those channels (no
// route names them) start with it on; channels a route names, and push (APNs)
// channels, keep exactly what they got before.
func schemaMigrationChannelAllAlerts() schemaMigration {
	return schemaMigration{Version: 112, Name: "notification_channel_all_alerts", Statements: []string{
		`ALTER TABLE notification_channels ADD COLUMN IF NOT EXISTS all_alerts BOOLEAN NOT NULL DEFAULT FALSE`,
		`UPDATE notification_channels c SET all_alerts = TRUE
		 WHERE c.type <> 'apns'
		   AND NOT EXISTS (
		     SELECT 1 FROM alert_routes r
		     WHERE jsonb_typeof(r.channel_ids) = 'array' AND r.channel_ids ? c.id)`,
	}}
}
