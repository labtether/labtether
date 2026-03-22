package main

import (
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/logs"
)

func (s *apiServer) appendAuditEventBestEffort(event audit.Event, logMessage string) {
	shared.AppendAuditEventBestEffort(s.auditStore, event, logMessage)
}

func (s *apiServer) appendLogEventBestEffort(event logs.Event, logMessage string) {
	shared.AppendLogEventBestEffort(s.logStore, event, logMessage)
}
