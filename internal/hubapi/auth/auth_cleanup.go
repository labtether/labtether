package auth

import (
	"context"
	"log"
	"time"

	"github.com/labtether/labtether/internal/persistence"
)

// RunSessionCleanupLoop periodically removes expired auth sessions.
func RunSessionCleanupLoop(ctx context.Context, store *persistence.PostgresStore) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deleted, err := store.DeleteExpiredSessions()
			if err != nil {
				log.Printf("labtether auth: session cleanup error: %v", err)
				continue
			}
			if deleted > 0 {
				log.Printf("labtether auth: cleaned up %d expired sessions", deleted)
			}
		}
	}
}
