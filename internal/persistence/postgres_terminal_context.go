package persistence

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Session lifecycle calls can hold a stream ownership gate. Bound both pool
// acquisition and I/O; the server statement timeout alone cannot bound either.
const terminalStoreQueryTimeout = 5 * time.Second

func terminalStoreContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), terminalStoreQueryTimeout)
}

func rollbackTerminalTransaction(tx pgx.Tx) {
	// Cleanup must still run after the operation deadline, but must not retain
	// the session gate indefinitely if the database has stopped responding.
	ctx, cancel := terminalStoreContext()
	defer cancel()
	_ = tx.Rollback(ctx)
}
