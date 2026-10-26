package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/incidents"
)

// These readers visit a complete scope from one owner snapshot, not event
// history. Visitors must be short, pure and must not reenter any store. Callers
// must discard all visited values on error. Separate reads are not one atomic
// cross-table snapshot; the worker must include scope values in retry identity.
type IncidentAssetScopeReader interface {
	ReadIncidentAssetsContext(context.Context, string, func(incidents.IncidentAsset) error) error
}

type IncidentAlertScopeReader interface {
	ReadIncidentAlertLinksContext(context.Context, string, func(incidents.AlertLink) error) error
}

type GroupAssetIDScopeReader interface {
	ReadGroupAssetIDsContext(context.Context, string, func(string) error) error
}

const incidentScopeReadTimeout = 5 * time.Second
const incidentScopePageSize = 128

func validIncidentScopeID(id string) bool {
	return id != "" && strings.TrimSpace(id) == id && !strings.ContainsRune(id, '\x00')
}

func validateIncidentAssetScope(row incidents.IncidentAsset, incidentID string) error {
	if !validIncidentScopeID(row.ID) || row.IncidentID != incidentID || !validIncidentScopeID(row.AssetID) {
		return fmt.Errorf("invalid incident asset scope row")
	}
	return nil
}

func validateIncidentAlertScope(row incidents.AlertLink, incidentID string) error {
	if !validIncidentScopeID(row.ID) || row.IncidentID != incidentID ||
		(row.AlertRuleID == "" && row.AlertInstanceID == "" && row.AlertFingerprint == "") {
		return fmt.Errorf("invalid incident alert scope row")
	}
	for _, id := range []string{row.AlertRuleID, row.AlertInstanceID, row.AlertFingerprint} {
		if id != "" && !validIncidentScopeID(id) {
			return fmt.Errorf("invalid incident alert scope discriminator")
		}
	}
	return nil
}
