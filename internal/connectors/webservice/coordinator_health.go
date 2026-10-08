package webservice

import (
	"github.com/labtether/labtether/internal/agentmgr"
	"math"
	"strings"
	"time"
)

type serviceHealthSample struct {
	at         time.Time
	status     string
	responseMs int
}

type serviceHealthHistory struct {
	samples []serviceHealthSample
}

// AttachHealthSummaries populates per-service rolling uptime and status history.
func (c *Coordinator) AttachHealthSummaries(services []agentmgr.DiscoveredWebService) {
	if len(services) == 0 {
		return
	}
	now := c.now()

	c.mu.RLock()
	defer c.mu.RUnlock()

	for index := range services {
		key := serviceOverrideKey(services[index].HostAssetID, services[index].ID)
		services[index].Health = buildServiceHealthSummary(c.serviceHealth[key], now)
	}
}

func cloneWebServiceHealthSummary(in *agentmgr.WebServiceHealthSummary) *agentmgr.WebServiceHealthSummary {
	if in == nil {
		return nil
	}
	out := *in
	if len(in.Recent) > 0 {
		out.Recent = append([]agentmgr.WebServiceHealthPoint(nil), in.Recent...)
	}
	return &out
}

func normalizeServiceStatus(status string) string {
	trimmed := strings.ToLower(strings.TrimSpace(status))
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}

func trimServiceHealthSamples(
	samples []serviceHealthSample,
	cutoff time.Time,
	maxSamples int,
) []serviceHealthSample {
	if len(samples) == 0 {
		return nil
	}
	start := 0
	for start < len(samples) && samples[start].at.Before(cutoff) {
		start++
	}
	if start >= len(samples) {
		return nil
	}
	trimmed := samples[start:]
	if maxSamples > 0 && len(trimmed) > maxSamples {
		trimmed = trimmed[len(trimmed)-maxSamples:]
	}
	return trimmed
}

func buildServiceHealthSummary(
	history *serviceHealthHistory,
	now time.Time,
) *agentmgr.WebServiceHealthSummary {
	if history == nil || len(history.samples) == 0 {
		return nil
	}
	cutoff := now.Add(-serviceHealthWindow)
	samples := trimServiceHealthSamples(history.samples, cutoff, 0)
	if len(samples) == 0 {
		return nil
	}

	upChecks := 0
	lastChangeAt := time.Time{}
	previousStatus := normalizeServiceStatus(samples[0].status)
	for index, sample := range samples {
		normalizedStatus := normalizeServiceStatus(sample.status)
		if strings.EqualFold(normalizedStatus, "up") {
			upChecks++
		}
		if index > 0 && normalizedStatus != previousStatus {
			lastChangeAt = sample.at
		}
		previousStatus = normalizedStatus
	}

	checks := len(samples)
	uptimePercent := 0.0
	if checks > 0 {
		uptimePercent = (float64(upChecks) / float64(checks)) * 100
		uptimePercent = math.Round(uptimePercent*10) / 10
	}

	recentStart := 0
	if len(samples) > serviceHealthRecentLimit {
		recentStart = len(samples) - serviceHealthRecentLimit
	}
	recent := make([]agentmgr.WebServiceHealthPoint, 0, len(samples)-recentStart)
	for _, sample := range samples[recentStart:] {
		recent = append(recent, agentmgr.WebServiceHealthPoint{
			At:         sample.at.UTC().Format(time.RFC3339),
			Status:     normalizeServiceStatus(sample.status),
			ResponseMs: sample.responseMs,
		})
	}

	summary := &agentmgr.WebServiceHealthSummary{
		Window:        serviceHealthWindowLabel,
		Checks:        checks,
		UpChecks:      upChecks,
		UptimePercent: uptimePercent,
		LastCheckedAt: samples[len(samples)-1].at.UTC().Format(time.RFC3339),
		Recent:        recent,
	}
	if !lastChangeAt.IsZero() {
		summary.LastChangeAt = lastChangeAt.UTC().Format(time.RFC3339)
	}
	return summary
}

func (c *Coordinator) recordServiceHealthSampleLocked(
	hostID string,
	serviceID string,
	status string,
	responseMs int,
	at time.Time,
) {
	if c == nil {
		return
	}
	hostID = strings.TrimSpace(hostID)
	serviceID = strings.TrimSpace(serviceID)
	if hostID == "" || serviceID == "" {
		return
	}
	key := serviceOverrideKey(hostID, serviceID)
	history, ok := c.serviceHealth[key]
	if !ok || history == nil {
		history = &serviceHealthHistory{}
		c.serviceHealth[key] = history
	}

	normalizedStatus := normalizeServiceStatus(status)
	if count := len(history.samples); count > 0 {
		last := &history.samples[count-1]
		if normalizeServiceStatus(last.status) == normalizedStatus && at.Sub(last.at) <= serviceHealthCoalesce {
			last.at = at
			last.status = normalizedStatus
			last.responseMs = responseMs
			return
		}
	}

	history.samples = append(history.samples, serviceHealthSample{
		at:         at,
		status:     normalizedStatus,
		responseMs: responseMs,
	})
	history.samples = trimServiceHealthSamples(history.samples, at.Add(-serviceHealthWindow), serviceHealthMaxSamples)
	if len(history.samples) == 0 {
		delete(c.serviceHealth, key)
	}
}

func (c *Coordinator) removeServiceHealthForHostLocked(hostID string) {
	if c == nil {
		return
	}
	trimmedHostID := strings.TrimSpace(hostID)
	if trimmedHostID == "" {
		return
	}
	prefix := trimmedHostID + "::"
	for key := range c.serviceHealth {
		if strings.HasPrefix(key, prefix) {
			delete(c.serviceHealth, key)
		}
	}
}

func (c *Coordinator) pruneServiceHealthLocked(now time.Time) {
	if c == nil {
		return
	}
	cutoff := now.Add(-serviceHealthWindow)
	for key, history := range c.serviceHealth {
		if history == nil {
			delete(c.serviceHealth, key)
			continue
		}
		history.samples = trimServiceHealthSamples(history.samples, cutoff, serviceHealthMaxSamples)
		if len(history.samples) == 0 {
			delete(c.serviceHealth, key)
		}
	}
}
