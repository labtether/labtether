package alerting

import (
	"context"
	"fmt"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/persistence"
	"log"
	"strings"
	"time"
)

func (d *Deps) MaybeAutoCreateIncident(rule alerts.Rule, instance alerts.AlertInstance) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d.maybeAutoCreateIncidentContext(ctx, rule, instance)
}

func (d *Deps) maybeAutoCreateIncidentContext(ctx context.Context, rule alerts.Rule, instance alerts.AlertInstance) {
	if d.IncidentStore == nil || ctx.Err() != nil {
		return
	}

	// Only auto-create for critical severity
	if rule.Severity != alerts.SeverityCritical {
		return
	}

	// Only if firing for > 5 minutes
	if time.Since(instance.StartedAt) < 5*time.Minute {
		return
	}

	store, ok := d.IncidentStore.(persistence.AlertAutoIncidentContextStore)
	if !ok {
		log.Printf("alert evaluator: context-aware auto incident store unavailable")
		return
	}
	// The store owns duplicate prevention (one open issue per rule) and
	// writes the issue, its alert link and the first timeline line together.
	title := rule.Name
	if device := d.autoIncidentDeviceName(instance); device != "" {
		title = fmt.Sprintf("%s on %s", rule.Name, device)
	}
	inc, created, err := store.CreateAutoIncidentForAlertInstanceContext(ctx, instance.ID, incidents.CreateIncidentRequest{
		Title:    title,
		Severity: incidents.SeverityCritical,
		Source:   incidents.SourceAlertAuto,
		Summary:  fmt.Sprintf("%s has been firing for 5 minutes.", rule.Name),
	})
	if err != nil {
		log.Printf("alert evaluator: failed to auto-create incident: %v", err)
		return
	}

	if !created || ctx.Err() != nil {
		return
	}
	d.dispatchIncidentNotificationAsync(inc, "incident.created")
	log.Printf("alert evaluator: auto-created incident %s from critical alert %s", inc.ID, rule.Name)
}

// autoIncidentDeviceName names the one device an alert fired for, or "".
func (d *Deps) autoIncidentDeviceName(instance alerts.AlertInstance) string {
	if d.AssetStore == nil || instance.FiringScope == nil || len(instance.FiringScope.AssetIDs) != 1 {
		return ""
	}
	asset, ok, err := d.AssetStore.GetAsset(instance.FiringScope.AssetIDs[0])
	if err != nil || !ok {
		return ""
	}
	return strings.TrimSpace(asset.Name)
}
