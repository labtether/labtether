package persistence

import (
	"context"
	"testing"
	"time"
)

// Separately gated real SQL proof. Memory cannot prove atomic concurrent UPDATE
// admission, nullable timestamp scanning, FK cleanup or prospective history.
func TestPostgresAlertTimingStateAndAtomicClaims(t *testing.T) {
	requireOwnedRecoveryDatabase(t)
	s := newTestPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var existing int
	if err := s.pool.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM alert_rules)+(SELECT COUNT(*) FROM alert_instances)+(SELECT COUNT(*) FROM alert_evaluations)`).Scan(&existing); err != nil || existing != 0 {
		t.Fatalf("requires empty owned alert tables: %d/%v", existing, err)
	}
	// Registered before shared exact-ID deletions, so this verifies their cascade.
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		var remains int
		if err := s.pool.QueryRow(cleanupCtx, `SELECT (SELECT COUNT(*) FROM alert_rules)+
 (SELECT COUNT(*) FROM alert_instances)+
 (SELECT COUNT(*) FROM alert_evaluations)`).Scan(&remains); err != nil || remains != 0 {
			t.Errorf("owned timing cleanup: %d/%v", remains, err)
		}
	})
	checkAlertTimingStores(t, s, s, func(id string) {
		if _, err := s.pool.Exec(ctx, `UPDATE alert_instances SET first_fired_at=NULL WHERE id=$1`, id); err != nil {
			t.Fatal(err)
		}
	})
}
