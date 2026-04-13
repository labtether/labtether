package statusagg

import (
	"context"
	"strings"
	"sync"

	"github.com/labtether/labtether/internal/actions"
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

	var (
		c  aggregateCollections
		wg sync.WaitGroup
	)
	c.assetGroup = assetGroup

	wg.Add(11)

	go func() {
		defer wg.Done()
		c.groups = d.listGroups()
	}()

	go func() {
		defer wg.Done()
		c.connectors = d.listConnectors()
	}()

	go func() {
		defer wg.Done()
		c.sessions = filterSessions(d.listSessions(), actorID)
	}()

	go func() {
		defer wg.Done()
		c.recentCommands = filterCommands(d.listRecentCommands(12), actorID)
	}()

	go func() {
		defer wg.Done()
		c.recentAudit = filterAuditEvents(d.listRecentAudit(20), actorID)
	}()

	go func() {
		defer wg.Done()
		c.recentLogs = d.listRecentLogs(groupFilter, assetGroup)
	}()

	go func() {
		defer wg.Done()
		c.logSources = d.listLogSources(groupFilter, assetGroup, caller)
	}()

	go func() {
		defer wg.Done()
		c.actionRuns = d.listActionRuns(groupFilter, assetGroup)
	}()

	go func() {
		defer wg.Done()
		c.updatePlans = d.listUpdatePlans(12)
	}()

	go func() {
		defer wg.Done()
		c.updateRuns = d.listUpdateRuns(groupFilter, assetGroup)
	}()

	go func() {
		defer wg.Done()
		deadLetters := d.loadDeadLetters()
		c.deadLetterEvents = deadLetters.Events
		c.deadLetterTotal = deadLetters.Total
		c.deadLetterStats = deadLetters.Analytics
	}()

	wg.Wait()
	c.groupReliability = d.listGroupReliability(c.groups, allAssets)
	return c
}

func filterSessions(sessions []terminal.Session, actorID string) []terminal.Session {
	if isOwnerActor(actorID) {
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

func filterCommands(commands []terminal.Command, actorID string) []terminal.Command {
	if isOwnerActor(actorID) {
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

func filterAuditEvents(events []audit.Event, actorID string) []audit.Event {
	if isOwnerActor(actorID) {
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
func terminalScopeKey(actorID string) string {
	if isOwnerActor(actorID) {
		return "owner"
	}
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return "actor:owner"
	}
	return "actor:" + actorID
}
