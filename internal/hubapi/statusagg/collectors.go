package statusagg

import (
	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/labtether/labtether/internal/updates"
	"log"
	"strings"
	"time"
)

const statusTelemetryOverviewCacheTTL = 10 * time.Second

// --- Asset helpers ---

// FilterAssetsByGroup returns assets whose GroupID matches groupFilter.
// If groupFilter is empty all assets are returned.
func FilterAssetsByGroup(assetList []assets.Asset, groupFilter string) []assets.Asset {
	groupFilter = strings.TrimSpace(groupFilter)
	if groupFilter == "" {
		if assetList == nil {
			return []assets.Asset{}
		}
		return assetList
	}
	filtered := make([]assets.Asset, 0, len(assetList))
	for _, assetEntry := range assetList {
		if strings.TrimSpace(assetEntry.GroupID) == groupFilter {
			filtered = append(filtered, assetEntry)
		}
	}
	return filtered
}

// AssetGroupLookup builds a map of assetID -> groupID for assets that have a
// non-empty GroupID.
func AssetGroupLookup(assetList []assets.Asset) map[string]string {
	assetGroup := make(map[string]string, len(assetList))
	for _, assetEntry := range assetList {
		groupID := strings.TrimSpace(assetEntry.GroupID)
		if groupID != "" {
			assetGroup[assetEntry.ID] = groupID
		}
	}
	return assetGroup
}

// CountStaleAssets returns the number of assets that are not "online" at now.
func CountStaleAssets(assetList []assets.Asset, now time.Time) int {
	count := 0
	for _, assetEntry := range assetList {
		if AssetFreshness(assetEntry.LastSeenAt, now) != "online" {
			count++
		}
	}
	return count
}

// AssetFreshness returns "online", "stale", or "offline" based on how recently
// the asset was last seen.
func AssetFreshness(lastSeenAt, now time.Time) string {
	if lastSeenAt.IsZero() {
		return "offline"
	}
	diff := now.Sub(lastSeenAt.UTC())
	if diff < 0 {
		return "offline"
	}
	if diff < 65*time.Second {
		return "online"
	}
	if diff < 5*time.Minute {
		return "stale"
	}
	return "offline"
}

// --- Web service summary ---

func (d *Deps) webServiceSummary(assetsFiltered []assets.Asset) (up int, total int) {
	if d.WebServiceCoordinator == nil {
		return 0, 0
	}

	allowedHosts := make(map[string]struct{}, len(assetsFiltered))
	for _, asset := range assetsFiltered {
		allowedHosts[asset.ID] = struct{}{}
	}
	return d.WebServiceCoordinator.SummaryByHosts(allowedHosts)
}

// --- Telemetry overview ---

func (d *Deps) buildTelemetryOverview(assetList []assets.Asset, now time.Time) []shared.AssetTelemetryOverview {
	out := make([]shared.AssetTelemetryOverview, 0, len(assetList))
	if d.TelemetryStore == nil {
		return out
	}

	if batchStore, ok := d.TelemetryStore.(persistence.TelemetrySnapshotBatchStore); ok {
		if cached, hit := d.telemetryOverviewCacheLookup(assetList, now); hit {
			return cached
		}

		assetIDs := CanonicalAssetIDs(assetList)
		snapshots, err := batchStore.SnapshotMany(assetIDs, now)
		if err == nil {
			for _, assetEntry := range assetList {
				metrics := snapshots[assetEntry.ID]
				out = append(out, shared.AssetTelemetryOverview{
					AssetID:    assetEntry.ID,
					Name:       assetEntry.Name,
					Type:       assetEntry.Type,
					Source:     assetEntry.Source,
					GroupID:    assetEntry.GroupID,
					Status:     assetEntry.Status,
					Platform:   assetEntry.Platform,
					LastSeenAt: assetEntry.LastSeenAt,
					Metrics:    metrics,
				})
			}
			d.telemetryOverviewCacheStore(assetList, now, out)
			return out
		}
		log.Printf("status aggregate: failed to batch query telemetry snapshots: %v", err)
	}

	for _, assetEntry := range assetList {
		metrics, err := d.TelemetryStore.Snapshot(assetEntry.ID, now)
		if err != nil {
			log.Printf("status aggregate: failed to query telemetry for %s: %v", assetEntry.ID, err)
			continue
		}
		out = append(out, shared.AssetTelemetryOverview{
			AssetID:    assetEntry.ID,
			Name:       assetEntry.Name,
			Type:       assetEntry.Type,
			Source:     assetEntry.Source,
			GroupID:    assetEntry.GroupID,
			Status:     assetEntry.Status,
			Platform:   assetEntry.Platform,
			LastSeenAt: assetEntry.LastSeenAt,
			Metrics:    metrics,
		})
	}
	d.telemetryOverviewCacheStore(assetList, now, out)
	return out
}

func (d *Deps) telemetryOverviewCacheLookup(
	assetList []assets.Asset,
	now time.Time,
) ([]shared.AssetTelemetryOverview, bool) {
	fingerprint := CanonicalAssetFingerprint(assetList)
	now = now.UTC()

	d.Cache.TelemetryOverviewCacheMu.RLock()
	entry := d.Cache.TelemetryOverviewCache
	d.Cache.TelemetryOverviewCacheMu.RUnlock()

	if entry.AssetFingerprint != fingerprint || !entry.ExpiresAt.After(now) {
		return nil, false
	}
	return append([]shared.AssetTelemetryOverview(nil), entry.Overview...), true
}

func (d *Deps) telemetryOverviewCacheStore(
	assetList []assets.Asset,
	now time.Time,
	overview []shared.AssetTelemetryOverview,
) {
	now = now.UTC()
	d.Cache.TelemetryOverviewCacheMu.Lock()
	d.Cache.TelemetryOverviewCache = TelemetryOverviewCacheEntry{
		AssetFingerprint: CanonicalAssetFingerprint(assetList),
		ExpiresAt:        now.Add(statusTelemetryOverviewCacheTTL),
		Overview:         append([]shared.AssetTelemetryOverview(nil), overview...),
	}
	d.Cache.TelemetryOverviewCacheMu.Unlock()
}

// --- List helpers ---

func (d *Deps) listAssets() []assets.Asset {
	if d.AssetStore == nil {
		return []assets.Asset{}
	}
	assetList, err := d.AssetStore.ListAssets()
	if err != nil {
		log.Printf("status aggregate: failed to list assets: %v", err)
		return []assets.Asset{}
	}
	if assetList == nil {
		return []assets.Asset{}
	}
	return assetList
}

func (d *Deps) listGroups() []groups.Group {
	if d.GroupStore == nil {
		return []groups.Group{}
	}
	groupList, err := d.GroupStore.ListGroups()
	if err != nil {
		log.Printf("status aggregate: failed to list groups: %v", err)
		return []groups.Group{}
	}
	if groupList == nil {
		return []groups.Group{}
	}
	return groupList
}

func (d *Deps) listSessions() []terminal.Session {
	if d.TerminalStore == nil {
		return []terminal.Session{}
	}
	sessions, err := d.TerminalStore.ListSessions()
	if err != nil {
		log.Printf("status aggregate: failed to list sessions: %v", err)
		return []terminal.Session{}
	}
	if sessions == nil {
		return []terminal.Session{}
	}
	return sessions
}

func (d *Deps) listRecentCommands(limit int) []terminal.Command {
	if d.TerminalStore == nil {
		return []terminal.Command{}
	}
	commands, err := d.TerminalStore.ListRecentCommands(limit)
	if err != nil {
		log.Printf("status aggregate: failed to list recent commands: %v", err)
		return []terminal.Command{}
	}
	if commands == nil {
		return []terminal.Command{}
	}
	return commands
}

func (d *Deps) listRecentAudit(limit int) []audit.Event {
	if d.AuditStore == nil {
		return []audit.Event{}
	}
	events, err := d.AuditStore.List(limit, 0)
	if err != nil {
		log.Printf("status aggregate: failed to list audit events: %v", err)
		return []audit.Event{}
	}
	if events == nil {
		return []audit.Event{}
	}
	return events
}

func (d *Deps) listConnectors() []connectorsdk.Descriptor {
	if d.ConnectorRegistry == nil {
		return []connectorsdk.Descriptor{}
	}
	connectors := d.ConnectorRegistry.List()
	if connectors == nil {
		return []connectorsdk.Descriptor{}
	}
	return connectors
}

func (d *Deps) listActionRuns(groupFilter string, assetGroup map[string]string) []actions.Run {
	if d.ActionStore == nil {
		return []actions.Run{}
	}
	runs, err := d.ActionStore.ListActionRuns(12, 0, "", "")
	if err != nil {
		log.Printf("status aggregate: failed to list action runs: %v", err)
		return []actions.Run{}
	}
	if groupFilter == "" {
		if runs == nil {
			return []actions.Run{}
		}
		return runs
	}
	filtered := make([]actions.Run, 0, len(runs))
	for _, run := range runs {
		if shared.ActionRunMatchesGroup(run, groupFilter, assetGroup) {
			filtered = append(filtered, run)
		}
	}
	return filtered
}

func (d *Deps) listUpdatePlans(limit int) []updates.Plan {
	if d.UpdateStore == nil {
		return []updates.Plan{}
	}
	plans, err := d.UpdateStore.ListUpdatePlans(limit)
	if err != nil {
		log.Printf("status aggregate: failed to list update plans: %v", err)
		return []updates.Plan{}
	}
	if plans == nil {
		return []updates.Plan{}
	}
	return plans
}

func (d *Deps) listUpdateRuns(groupFilter string, assetGroup map[string]string) []updates.Run {
	if d.UpdateStore == nil {
		return []updates.Run{}
	}
	runs, err := d.UpdateStore.ListUpdateRuns(12, "")
	if err != nil {
		log.Printf("status aggregate: failed to list update runs: %v", err)
		return []updates.Run{}
	}
	if groupFilter == "" {
		if runs == nil {
			return []updates.Run{}
		}
		return runs
	}

	planIDs := make([]string, 0, len(runs))
	for _, run := range runs {
		planID := strings.TrimSpace(run.PlanID)
		if planID == "" {
			continue
		}
		planIDs = append(planIDs, planID)
	}
	plansByID, err := d.loadUpdatePlansByID(planIDs)
	if err != nil {
		log.Printf("status aggregate: failed to bulk-load update plans for group filter: %v", err)
	}

	planGroupCache := make(map[string]bool, len(runs))
	filtered := make([]updates.Run, 0, len(runs))
	for _, run := range runs {
		planID := strings.TrimSpace(run.PlanID)
		if planID == "" {
			continue
		}

		touchesGroup := false
		if plan, ok := plansByID[planID]; ok {
			touchesGroup = shared.UpdatePlanTouchesGroup(plan, groupFilter, assetGroup)
		} else {
			var touchErr error
			touchesGroup, touchErr = d.updateRunTouchesGroup(run.PlanID, groupFilter, assetGroup, planGroupCache)
			if touchErr != nil {
				log.Printf("status aggregate: failed to map update run %s to group: %v", run.ID, touchErr)
				continue
			}
		}

		if touchesGroup {
			filtered = append(filtered, run)
		}
	}
	return filtered
}
