package persistence

import (
	"strings"
	"testing"
	"time"
)

func TestDurablePushDigestMigrationHasBoundedMultiHubState(t *testing.T) {
	var migration *schemaMigration
	migrations := postgresSchemaMigrations()
	for index := range migrations {
		candidate := &migrations[index]
		if candidate.Version == 91 {
			migration = candidate
			break
		}
	}
	if migration == nil {
		t.Fatal("durable push digest migration v91 is missing")
	}
	joined := strings.Join(migration.Statements, "\n")
	for _, required := range []string{
		"push_alert_digest_states",
		"push_alert_digest_events",
		"REFERENCES push_devices(id) ON DELETE CASCADE",
		"REFERENCES notification_channels(id) ON DELETE CASCADE",
		"window_seconds BETWEEN 30 AND 86400",
		"retry_count BETWEEN 0 AND 8",
		"delivery_generation",
		"lease_expires_at",
		"idx_push_alert_digest_states_due",
		"idx_push_alert_digest_states_expiry",
		"UNIQUE(push_device_id, dedupe_key)",
		"dedupe_key ~ '^[0-9a-f]{64}$'",
		"CHECK (severity IN ('info', 'warning'))",
		"jsonb_typeof(group_ids) = 'array'",
		"idx_push_alert_digest_events_device_time",
		"idx_push_alert_digest_events_expiry",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("migration v91 missing %q", required)
		}
	}
}

func TestPushDigestTTLLeavesDeliveryAndRetryHeadroomAtMaximumWindow(t *testing.T) {
	maximumWindow := 24 * time.Hour
	if PushDigestEventTTL <= maximumWindow+time.Minute {
		t.Fatalf("push digest TTL %s does not leave worker/retry headroom after maximum window %s", PushDigestEventTTL, maximumWindow)
	}
}

func TestNormalizePushDigestEnqueueBoundsAndFailsClosedOnLargeMaintenanceScope(t *testing.T) {
	groups := make([]string, MaxPushDigestGroupIDs+1)
	for index := range groups {
		groups[index] = "group-" + strings.Repeat("a", index%5) + string(rune('A'+index%26))
	}
	event, err := normalizePushDigestEnqueue(PushDigestEnqueue{
		DeviceID: "device", ChannelID: "channel",
		DedupeKey: strings.Repeat("a", 64), Severity: "warning",
		GroupIDs: groups, MaintenanceScopeComplete: true, WindowSeconds: 1,
	})
	if err != nil {
		t.Fatalf("normalize digest enqueue: %v", err)
	}
	if event.WindowSeconds != 30 {
		t.Fatalf("window = %d, want lower bound 30", event.WindowSeconds)
	}
	if event.MaintenanceScopeComplete || len(event.GroupIDs) != 0 {
		t.Fatalf("oversized maintenance scope did not fail closed: %+v", event)
	}
}
