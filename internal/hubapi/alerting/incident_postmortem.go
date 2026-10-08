package alerting

import (
	"fmt"
	"github.com/labtether/labtether/internal/incidents"
	"strings"
	"time"
)

func buildIncidentPostmortem(inc incidents.Incident, alertLinks []incidents.AlertLink) string {
	var b strings.Builder
	const timeFmt = "2006-01-02 15:04:05 UTC"

	b.WriteString(fmt.Sprintf("# Incident: %s\n\n", inc.Title))

	// Summary
	b.WriteString("## Summary\n\n")
	b.WriteString(fmt.Sprintf("- **Severity**: %s\n", inc.Severity))
	b.WriteString(fmt.Sprintf("- **Status**: %s\n", inc.Status))
	b.WriteString(fmt.Sprintf("- **Source**: %s\n", inc.Source))
	if inc.Summary != "" {
		b.WriteString(fmt.Sprintf("\n%s\n", inc.Summary))
	}
	b.WriteString("\n")

	// Timeline
	b.WriteString("## Timeline\n\n")
	b.WriteString(fmt.Sprintf("- **Opened**: %s\n", inc.OpenedAt.UTC().Format(timeFmt)))
	if inc.MitigatedAt != nil {
		b.WriteString(fmt.Sprintf("- **Mitigated**: %s\n", inc.MitigatedAt.UTC().Format(timeFmt)))
	}
	if inc.ResolvedAt != nil {
		b.WriteString(fmt.Sprintf("- **Resolved**: %s\n", inc.ResolvedAt.UTC().Format(timeFmt)))
	}
	if inc.ClosedAt != nil {
		b.WriteString(fmt.Sprintf("- **Closed**: %s\n", inc.ClosedAt.UTC().Format(timeFmt)))
	}
	b.WriteString("\n")

	// Impact
	b.WriteString("## Impact\n\n")
	if inc.GroupID != "" {
		b.WriteString(fmt.Sprintf("- **Group**: %s\n", inc.GroupID))
	}
	if inc.PrimaryAssetID != "" {
		b.WriteString(fmt.Sprintf("- **Primary Asset**: %s\n", inc.PrimaryAssetID))
	}
	if inc.Assignee != "" {
		b.WriteString(fmt.Sprintf("- **Assignee**: %s\n", inc.Assignee))
	}
	b.WriteString("\n")

	// Root Cause
	b.WriteString("## Root Cause\n\n")
	if inc.RootCause != "" {
		b.WriteString(inc.RootCause + "\n")
	} else {
		b.WriteString("_Not yet documented._\n")
	}
	b.WriteString("\n")

	// Action Items
	b.WriteString("## Action Items\n\n")
	if len(inc.ActionItems) > 0 {
		for _, item := range inc.ActionItems {
			b.WriteString(fmt.Sprintf("- %s\n", item))
		}
	} else {
		b.WriteString("_No action items recorded._\n")
	}
	b.WriteString("\n")

	// Lessons Learned
	b.WriteString("## Lessons Learned\n\n")
	if inc.LessonsLearned != "" {
		b.WriteString(inc.LessonsLearned + "\n")
	} else {
		b.WriteString("_No lessons learned recorded._\n")
	}
	b.WriteString("\n")

	// Metrics
	b.WriteString("## Metrics\n\n")
	if inc.ResolvedAt != nil {
		mttr := inc.ResolvedAt.Sub(inc.OpenedAt)
		b.WriteString(fmt.Sprintf("- **MTTR (Mean Time to Resolve)**: %s\n", formatDuration(mttr)))
	} else {
		b.WriteString("_Incident not yet resolved — MTTR unavailable._\n")
	}
	b.WriteString("\n")

	// Linked Alerts
	b.WriteString("## Linked Alerts\n\n")
	if len(alertLinks) > 0 {
		for _, link := range alertLinks {
			label := link.AlertRuleID
			if label == "" {
				label = link.AlertFingerprint
			}
			if label == "" {
				label = link.AlertInstanceID
			}
			b.WriteString(fmt.Sprintf("- %s (type: %s)\n", label, link.LinkType))
		}
	} else {
		b.WriteString("_No linked alerts._\n")
	}
	b.WriteString("\n")

	return b.String()
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if hours < 24 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	days := hours / 24
	hours = hours % 24
	return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
}
