package statusagg

import (
	"context"
	"sort"
	"strings"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groups"
	groupfeatures "github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/labtether/labtether/internal/updates"
)

func statusScopeKey(ctx context.Context) string {
	parts := []string{terminalScopeKey(ctx)}
	scopes := append([]string(nil), apiv2.ScopesFromContext(ctx)...)
	allowedAssets := append([]string(nil), apiv2.AllowedAssetsFromContext(ctx)...)
	sort.Strings(scopes)
	sort.Strings(allowedAssets)
	parts = append(parts, "scopes="+strings.Join(scopes, ","), "assets="+strings.Join(allowedAssets, ","))
	return Fingerprint(parts)
}

func statusHasScope(ctx context.Context, scope string) bool {
	return apiv2.ScopeCheck(apiv2.ScopesFromContext(ctx), scope)
}

func statusHasAssetRestriction(ctx context.Context) bool {
	return len(apiv2.AllowedAssetsFromContext(ctx)) > 0
}

func filterStatusAssets(ctx context.Context, assetList []assets.Asset) []assets.Asset {
	if !statusHasScope(ctx, "assets:read") {
		return []assets.Asset{}
	}
	allowed := apiv2.AllowedAssetsFromContext(ctx)
	if len(allowed) == 0 {
		if assetList == nil {
			return []assets.Asset{}
		}
		return assetList
	}
	filtered := make([]assets.Asset, 0, len(assetList))
	for _, assetEntry := range assetList {
		if apiv2.AssetCheck(allowed, assetEntry.ID) {
			filtered = append(filtered, assetEntry)
		}
	}
	return filtered
}

func filterStatusGroups(ctx context.Context, groupList []groups.Group, accessibleAssets []assets.Asset) []groups.Group {
	if !statusHasScope(ctx, "groups:read") {
		return []groups.Group{}
	}
	if !statusHasAssetRestriction(ctx) {
		if groupList == nil {
			return []groups.Group{}
		}
		return groupList
	}

	groupsByID := make(map[string]groups.Group, len(groupList))
	visible := make(map[string]struct{}, len(accessibleAssets))
	for _, groupEntry := range groupList {
		groupsByID[strings.TrimSpace(groupEntry.ID)] = groupEntry
	}
	for _, assetEntry := range accessibleAssets {
		groupID := strings.TrimSpace(assetEntry.GroupID)
		for groupID != "" {
			if _, seen := visible[groupID]; seen {
				break
			}
			visible[groupID] = struct{}{}
			parent, ok := groupsByID[groupID]
			if !ok {
				break
			}
			groupID = strings.TrimSpace(parent.ParentGroupID)
		}
	}

	filtered := make([]groups.Group, 0, len(visible))
	for _, groupEntry := range groupList {
		if _, ok := visible[strings.TrimSpace(groupEntry.ID)]; ok {
			filtered = append(filtered, groupEntry)
		}
	}
	return filtered
}

func filterStatusLogs(events []logs.Event, allowedAssetIDs map[string]struct{}) []logs.Event {
	filtered := make([]logs.Event, 0, len(events))
	for _, event := range events {
		if _, ok := allowedAssetIDs[strings.TrimSpace(event.AssetID)]; ok {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func filterStatusActionRuns(runs []actions.Run, allowedAssetIDs map[string]struct{}) []actions.Run {
	filtered := make([]actions.Run, 0, len(runs))
	for _, run := range runs {
		if _, ok := allowedAssetIDs[strings.TrimSpace(run.Target)]; ok {
			filtered = append(filtered, run)
		}
	}
	return filtered
}

func filterStatusUpdatePlans(plans []updates.Plan, allowedAssetIDs map[string]struct{}) ([]updates.Plan, map[string]struct{}) {
	filtered := make([]updates.Plan, 0, len(plans))
	allowedPlanIDs := make(map[string]struct{}, len(plans))
	for _, plan := range plans {
		if len(plan.Targets) == 0 {
			continue
		}
		allowed := true
		for _, target := range plan.Targets {
			if _, ok := allowedAssetIDs[strings.TrimSpace(target)]; !ok {
				allowed = false
				break
			}
		}
		if allowed {
			filtered = append(filtered, plan)
			allowedPlanIDs[strings.TrimSpace(plan.ID)] = struct{}{}
		}
	}
	return filtered, allowedPlanIDs
}

func filterStatusUpdateRuns(runs []updates.Run, allowedPlanIDs map[string]struct{}, allowedAssetIDs map[string]struct{}) []updates.Run {
	filtered := make([]updates.Run, 0, len(runs))
	for _, run := range runs {
		if _, ok := allowedPlanIDs[strings.TrimSpace(run.PlanID)]; !ok {
			continue
		}
		copyRun := run
		copyRun.Results = make([]updates.RunResultEntry, 0, len(run.Results))
		for _, result := range run.Results {
			if _, ok := allowedAssetIDs[strings.TrimSpace(result.Target)]; ok {
				copyRun.Results = append(copyRun.Results, result)
			}
		}
		filtered = append(filtered, copyRun)
	}
	return filtered
}

func filterStatusSessions(sessions []terminal.Session, allowedAssetIDs map[string]struct{}) []terminal.Session {
	filtered := make([]terminal.Session, 0, len(sessions))
	for _, session := range sessions {
		if _, ok := allowedAssetIDs[strings.TrimSpace(session.Target)]; ok {
			filtered = append(filtered, session)
		}
	}
	return filtered
}

func filterStatusCommands(commands []terminal.Command, allowedAssetIDs map[string]struct{}) []terminal.Command {
	filtered := make([]terminal.Command, 0, len(commands))
	for _, command := range commands {
		if _, ok := allowedAssetIDs[strings.TrimSpace(command.Target)]; ok {
			filtered = append(filtered, command)
		}
	}
	return filtered
}

func filterStatusReliability(records []groupfeatures.GroupReliabilityRecord, allowedGroupIDs map[string]struct{}) []groupfeatures.GroupReliabilityRecord {
	filtered := make([]groupfeatures.GroupReliabilityRecord, 0, len(records))
	for _, record := range records {
		if _, ok := allowedGroupIDs[strings.TrimSpace(record.Group.ID)]; ok {
			filtered = append(filtered, record)
		}
	}
	return filtered
}
