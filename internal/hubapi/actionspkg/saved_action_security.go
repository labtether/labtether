package actionspkg

import (
	"context"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/savedactions"
)

const maxSavedActionIDLength = 255

func validSavedActionID(id string) bool {
	if id == "" || len(id) > maxSavedActionIDLength || !utf8.ValidString(id) || strings.TrimSpace(id) != id {
		return false
	}
	for _, r := range id {
		if r < 0x20 || r == 0x7f || r == '/' || r == '\\' {
			return false
		}
	}
	return true
}

func writeSavedActionNotFound(w http.ResponseWriter) {
	apiv2.WriteError(w, http.StatusNotFound, "not_found", "saved action not found")
}

func (d *Deps) savedActionAssetIndex() (map[string]struct{}, error) {
	assetList, err := d.AssetStore.ListAssets()
	if err != nil {
		return nil, err
	}
	assetsByID := make(map[string]struct{}, len(assetList))
	for _, asset := range assetList {
		if id := strings.TrimSpace(asset.ID); id != "" {
			assetsByID[id] = struct{}{}
		}
	}
	return assetsByID, nil
}

func savedActionAccessible(ctx context.Context, action savedactions.SavedAction, assetsByID map[string]struct{}) bool {
	if len(action.Steps) == 0 || len(action.Steps) > maxSavedActionStepCount {
		return false
	}
	for _, step := range action.Steps {
		target := strings.TrimSpace(step.Target)
		if target == "" || target != step.Target || !apiv2.AssetCheckContext(ctx, target) {
			return false
		}
		if _, ok := assetsByID[target]; !ok {
			return false
		}
	}
	return true
}

func (d *Deps) getAccessibleSavedAction(r *http.Request, id string) (savedactions.SavedAction, bool, error) {
	action, ok, err := d.SavedActionStore.GetSavedAction(r.Context(), apiv2.PrincipalActorID(r.Context()), id)
	if err != nil || !ok {
		return savedactions.SavedAction{}, ok, err
	}
	assetsByID, err := d.savedActionAssetIndex()
	if err != nil {
		return savedactions.SavedAction{}, false, err
	}
	if !savedActionAccessible(r.Context(), action, assetsByID) {
		return savedactions.SavedAction{}, false, nil
	}
	return action, true, nil
}

func (d *Deps) auditSavedAction(r *http.Request, eventType string, action savedactions.SavedAction, decision, reason string, details map[string]any) {
	if d.AppendAuditEventBestEffort == nil {
		return
	}
	redacted := map[string]any{
		"step_count":   len(action.Steps),
		"target_count": savedActionTargetCount(action),
	}
	for key, value := range details {
		redacted[key] = value
	}
	event := audit.NewEvent(eventType)
	event.ActorID = apiv2.PrincipalActorID(r.Context())
	event.Target = action.ID
	event.Decision = decision
	event.Reason = reason
	event.Details = redacted
	d.AppendAuditEventBestEffort(event, "api warning: failed to append saved action audit event")
}

func savedActionTargetCount(action savedactions.SavedAction) int {
	targets := make(map[string]struct{}, len(action.Steps))
	for _, step := range action.Steps {
		if target := strings.TrimSpace(step.Target); target != "" {
			targets[target] = struct{}{}
		}
	}
	return len(targets)
}
