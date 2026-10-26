package persistence

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Agent callbacks run under the asset ownership gate. Bound pool acquisition
// and database I/O so a failed database cannot prevent socket replacement.
// This is a per-store-operation deadline; a callback may perform several calls.
const agentLifecycleStoreQueryTimeout = 5 * time.Second

func agentLifecycleStoreContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), agentLifecycleStoreQueryTimeout)
}

func rollbackAgentLifecycleTransaction(tx pgx.Tx) {
	// The operation context may already have expired. Give transaction cleanup
	// its own bound so it cannot retain the asset gate indefinitely either.
	ctx, cancel := agentLifecycleStoreContext()
	defer cancel()
	_ = tx.Rollback(ctx)
}
