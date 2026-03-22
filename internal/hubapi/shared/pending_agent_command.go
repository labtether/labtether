package shared

import (
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
)

// PendingAgentCommand holds the correlation state for an in-flight
// command sent to a connected agent via WebSocket. The hub stores these
// in a sync.Map keyed by job ID and matches incoming results back using
// AcceptsResult.
type PendingAgentCommand struct {
	ResultCh          chan agentmgr.CommandResultData
	ExpectedAssetID   string
	ExpectedSessionID string
	ExpectedCommandID string
}

// AcceptsResult returns true when a received command result matches the
// expectations stored in this pending command entry.
func (p PendingAgentCommand) AcceptsResult(conn *agentmgr.AgentConn, result *agentmgr.CommandResultData) bool {
	if result == nil {
		return false
	}
	if strings.TrimSpace(result.JobID) == "" {
		return false
	}

	expectedAssetID := strings.TrimSpace(p.ExpectedAssetID)
	if expectedAssetID != "" {
		if conn == nil || !strings.EqualFold(expectedAssetID, strings.TrimSpace(conn.AssetID)) {
			return false
		}
	}

	expectedSessionID := strings.TrimSpace(p.ExpectedSessionID)
	if expectedSessionID != "" {
		if actual := strings.TrimSpace(result.SessionID); actual == "" {
			result.SessionID = expectedSessionID
		} else if actual != expectedSessionID {
			return false
		}
	}

	expectedCommandID := strings.TrimSpace(p.ExpectedCommandID)
	if expectedCommandID != "" {
		if actual := strings.TrimSpace(result.CommandID); actual == "" {
			result.CommandID = expectedCommandID
		} else if actual != expectedCommandID {
			return false
		}
	}

	return true
}
