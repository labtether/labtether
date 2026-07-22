package schedulespkg

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/apikeys"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/schedules"
)

func (d *Deps) scheduleAllowed(ctx context.Context, task schedules.ScheduledTask) (bool, error) {
	if !shared.HasAssetRestriction(ctx) {
		return true, nil
	}
	hasConcreteScope := false
	var groupAccess map[string]struct{}
	loadGroups := func() error {
		if groupAccess != nil {
			return nil
		}
		if d.AssetStore == nil || d.GroupStore == nil {
			return fmt.Errorf("asset authorization stores unavailable")
		}
		groups, err := d.GroupStore.ListGroups()
		if err != nil {
			return err
		}
		assets, err := d.AssetStore.ListAssets()
		if err != nil {
			return err
		}
		groupAccess = shared.AccessibleGroupIDs(ctx, groups, assets)
		return nil
	}

	if groupID := strings.TrimSpace(task.GroupID); groupID != "" {
		hasConcreteScope = true
		if err := loadGroups(); err != nil {
			return false, err
		}
		if _, ok := groupAccess[groupID]; !ok {
			return false, nil
		}
	}
	for _, target := range task.Targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		hasConcreteScope = true
		if apiv2.AssetCheckContext(ctx, target) {
			continue
		}
		if err := loadGroups(); err != nil {
			return false, err
		}
		if _, ok := groupAccess[target]; !ok {
			return false, nil
		}
	}
	return hasConcreteScope, nil
}

// FilterScheduledTasksForAccess applies the HTTP schedule-list authorization
// contract to internal consumers such as MCP without duplicating group logic.
func (d *Deps) FilterScheduledTasksForAccess(ctx context.Context, tasks []schedules.ScheduledTask) ([]schedules.ScheduledTask, error) {
	if !shared.HasAssetRestriction(ctx) {
		return tasks, nil
	}
	filtered := make([]schedules.ScheduledTask, 0, len(tasks))
	for _, task := range tasks {
		allowed, err := d.scheduleAllowed(ctx, task)
		if err != nil {
			return nil, err
		}
		if allowed {
			filtered = append(filtered, task)
		}
	}
	return filtered, nil
}

// AuthorizeExecutionTarget revalidates the schedule creator at execution time.
// This prevents deleted/demoted users, expired or under-scoped API keys, and
// changed API-key asset allowlists from retaining latent command authority.
func (d *Deps) AuthorizeExecutionTarget(ctx context.Context, actorID, assetID string) error {
	actorID = strings.TrimSpace(actorID)
	assetID = strings.TrimSpace(assetID)
	if actorID == "owner" {
		return nil
	}
	if actorID == "" || assetID == "" {
		return fmt.Errorf("schedule creator or target is missing")
	}
	if keyID, ok := strings.CutPrefix(actorID, "apikey:"); ok {
		if d.APIKeyStore == nil {
			return fmt.Errorf("API key authorization store unavailable")
		}
		key, found, err := d.APIKeyStore.GetAPIKey(ctx, strings.TrimSpace(keyID))
		if err != nil {
			return fmt.Errorf("load schedule API key: %w", err)
		}
		if !found {
			return fmt.Errorf("schedule API key no longer exists")
		}
		if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now().UTC()) {
			return fmt.Errorf("schedule API key is expired")
		}
		if auth.NormalizeRole(key.Role) == auth.RoleOwner {
			return fmt.Errorf("schedule API key has reserved owner role")
		}
		if !auth.HasWritePrivileges(key.Role) {
			return fmt.Errorf("schedule API key no longer has write privileges")
		}
		if !apikeys.ScopeAllows(key.Scopes, "actions:exec") {
			return fmt.Errorf("schedule API key lacks actions:exec")
		}
		if !apikeys.AssetAllowed(key.AllowedAssets, assetID) {
			return fmt.Errorf("schedule API key no longer has access to target")
		}
		return nil
	}
	if d.AuthStore == nil {
		return fmt.Errorf("user authorization store unavailable")
	}
	user, found, err := d.AuthStore.GetUserByID(actorID)
	if err != nil {
		return fmt.Errorf("load schedule creator: %w", err)
	}
	if !found {
		return fmt.Errorf("schedule creator no longer exists")
	}
	if !auth.HasWritePrivileges(user.Role) {
		return fmt.Errorf("schedule creator no longer has write privileges")
	}
	return nil
}

func (d *Deps) requireScheduleAccess(w http.ResponseWriter, r *http.Request, task schedules.ScheduledTask) bool {
	allowed, err := d.scheduleAllowed(r.Context(), task)
	if err == nil && allowed {
		return true
	}
	apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "api key does not have access to every asset targeted by this schedule")
	return false
}
