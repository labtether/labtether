package statusagg

import (
	"context"
	"strings"
	"sync"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/groups"
	groupfeatures "github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/labtether/labtether/internal/updates"
)

// aggregateCollections holds all data assembled concurrently for the full
// status aggregate response.
type aggregateCollections struct {
	assetGroup       map[string]string
	groups           []groups.Group
	connectors       []connectorsdk.Descriptor
	sessions         []terminal.Session
	recentCommands   []terminal.Command
	recentAudit      []audit.Event
	recentLogs       []logs.Event
	logSources       []logs.SourceSummary
	groupReliability []groupfeatures.GroupReliabilityRecord
	actionRuns       []actions.Run
	updatePlans      []updates.Plan
	updateRuns       []updates.Run
	deadLetterEvents []shared.DeadLetterEventResponse
	deadLetterTotal  int
	deadLetterStats  shared.DeadLetterAnalyticsResponse
}

func (d *Deps) collectAggregateCollections(
	ctx context.Context,
	groupFilter string,
	caller string,
	allAssets []assets.Asset,
) aggregateCollections {
	// These two are pure in-memory operations and must complete before the
	// goroutines below start, since several collectors accept assetGroup as
	// a parameter and actorID is used to filter their results.
	assetGroup := AssetGroupLookup(allAssets)
	actorID := principalActorID(ctx)
	ownerPrincipal := apiv2.IsOwnerPrincipal(ctx)
	restrictedAssets := statusHasAssetRestriction(ctx)
	canReadGroups := statusHasScope(ctx, "groups:read")
	canReadConnectors := statusHasScope(ctx, "connectors:read")
	canReadTerminal := statusHasScope(ctx, "terminal:read")
	canReadAudit := statusHasScope(ctx, "audit:read")
	canReadLogs := statusHasScope(ctx, "logs:read")
	canReadActions := statusHasScope(ctx, "actions:read")
	canReadUpdates := statusHasScope(ctx, "updates:read")
	canReadDeadLetters := statusHasScope(ctx, "dead-letters:read")

	var (
		c  aggregateCollections
		wg sync.WaitGroup
	)
	c.assetGroup = assetGroup
	c.groups = []groups.Group{}
	c.connectors = []connectorsdk.Descriptor{}
	c.sessions = []terminal.Session{}
	c.recentCommands = []terminal.Command{}
	c.recentAudit = []audit.Event{}
	c.recentLogs = []logs.Event{}
	c.logSources = []logs.SourceSummary{}
	c.groupReliability = []groupfeatures.GroupReliabilityRecord{}
	c.actionRuns = []actions.Run{}
	c.updatePlans = []updates.Plan{}
	c.updateRuns = []updates.Run{}
	c.deadLetterEvents = []shared.DeadLetterEventResponse{}
	c.deadLetterStats = shared.DeadLetterAnalyticsResponse{}

	wg.Add(11)

	go func() {
		defer wg.Done()
		if canReadGroups {
			c.groups = filterStatusGroups(ctx, d.listGroups(), allAssets)
		}
	}()

	go func() {
		defer wg.Done()
		if canReadConnectors {
			c.connectors = d.listConnectors()
		}
	}()

	go func() {
		defer wg.Done()
		if canReadTerminal {
			c.sessions = filterSessions(d.listSessions(), actorID, ownerPrincipal)
		}
	}()

	go func() {
		defer wg.Done()
		if canReadTerminal {
			c.recentCommands = filterCommands(d.listRecentCommands(12), actorID, ownerPrincipal)
		}
	}()

	go func() {
		defer wg.Done()
		if canReadAudit && !restrictedAssets {
			c.recentAudit = filterAuditEvents(d.listRecentAudit(20), actorID, ownerPrincipal)
		}
	}()

	go func() {
		defer wg.Done()
		if canReadLogs {
			c.recentLogs = d.listRecentLogs(groupFilter, assetGroup)
		}
	}()

	go func() {
		defer wg.Done()
		if canReadLogs && !restrictedAssets {
			c.logSources = d.listLogSources(groupFilter, assetGroup, caller)
		}
	}()

	go func() {
		defer wg.Done()
		if canReadActions {
			c.actionRuns = d.listActionRuns(groupFilter, assetGroup)
		}
	}()

	go func() {
		defer wg.Done()
		if canReadUpdates {
			c.updatePlans = d.listUpdatePlans(12)
		}
	}()

	go func() {
		defer wg.Done()
		if canReadUpdates {
			c.updateRuns = d.listUpdateRuns(groupFilter, assetGroup)
		}
	}()

	go func() {
		defer wg.Done()
		if canReadDeadLetters && !restrictedAssets {
			deadLetters := d.loadDeadLetters()
			c.deadLetterEvents = deadLetters.Events
			c.deadLetterTotal = deadLetters.Total
			c.deadLetterStats = deadLetters.Analytics
		}
	}()

	wg.Wait()
	if canReadGroups && canReadLogs && canReadActions && canReadUpdates && statusHasScope(ctx, "metrics:read") && !restrictedAssets {
		c.groupReliability = d.listGroupReliability(c.groups, allAssets)
	}

	if restrictedAssets {
		allowedAssetIDs := make(map[string]struct{}, len(allAssets))
		for _, assetEntry := range allAssets {
			allowedAssetIDs[strings.TrimSpace(assetEntry.ID)] = struct{}{}
		}
		allowedGroupIDs := make(map[string]struct{}, len(c.groups))
		for _, groupEntry := range c.groups {
			allowedGroupIDs[strings.TrimSpace(groupEntry.ID)] = struct{}{}
		}
		c.recentLogs = filterStatusLogs(c.recentLogs, allowedAssetIDs)
		c.actionRuns = filterStatusActionRuns(c.actionRuns, allowedAssetIDs)
		var allowedPlanIDs map[string]struct{}
		c.updatePlans, allowedPlanIDs = filterStatusUpdatePlans(c.updatePlans, allowedAssetIDs)
		c.updateRuns = filterStatusUpdateRuns(c.updateRuns, allowedPlanIDs, allowedAssetIDs)
		c.sessions = filterStatusSessions(c.sessions, allowedAssetIDs)
		c.recentCommands = filterStatusCommands(c.recentCommands, allowedAssetIDs)
		c.groupReliability = filterStatusReliability(c.groupReliability, allowedGroupIDs)
	}
	return c
}

func filterSessions(sessions []terminal.Session, actorID string, ownerPrincipal bool) []terminal.Session {
	if ownerPrincipal {
		return sessions
	}
	filtered := make([]terminal.Session, 0, len(sessions))
	for _, session := range sessions {
		if strings.TrimSpace(session.ActorID) == actorID {
			filtered = append(filtered, session)
		}
	}
	return filtered
}

func filterCommands(commands []terminal.Command, actorID string, ownerPrincipal bool) []terminal.Command {
	if ownerPrincipal {
		return commands
	}
	filtered := make([]terminal.Command, 0, len(commands))
	for _, command := range commands {
		if strings.TrimSpace(command.ActorID) == actorID {
			filtered = append(filtered, command)
		}
	}
	return filtered
}

func filterAuditEvents(events []audit.Event, actorID string, ownerPrincipal bool) []audit.Event {
	if ownerPrincipal {
		return events
	}
	filtered := make([]audit.Event, 0, len(events))
	for _, event := range events {
		if strings.TrimSpace(event.ActorID) == actorID {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// terminalScopeKey returns a cache scope key that differentiates owner
// sessions from per-actor sessions. It is used as part of the singleflight
// key so two simultaneous requests from different actors never share results.
func terminalScopeKey(ctx context.Context) string {
	if apiv2.IsOwnerPrincipal(ctx) {
		return "owner"
	}
	actorID := principalActorID(ctx)
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return "actor:owner"
	}
	return "actor:" + actorID
}
