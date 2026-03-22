package shared

import (
	"log"

	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
)

// AppendAuditEventBestEffort appends an audit event, logging on failure.
func AppendAuditEventBestEffort(store persistence.AuditStore, event audit.Event, logMessage string) {
	if store == nil {
		return
	}
	if err := store.Append(event); err != nil {
		log.Printf("%s: %v", logMessage, err)
	}
}

// AppendLogEventBestEffort appends a log event, logging on failure.
func AppendLogEventBestEffort(store persistence.LogStore, event logs.Event, logMessage string) {
	if store == nil {
		return
	}
	if err := store.AppendEvent(event); err != nil {
		log.Printf("%s: %v", logMessage, err)
	}
}
