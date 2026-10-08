package webservice

import (
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/persistence"
	"log"
	"strings"
)

type serviceSummaryEntry struct {
	hostAssetID string
	serviceID   string
	status      string
	serviceKey  string
	component   string
	hidden      bool
}

// SummaryByHosts returns visible service up/total counts for one or more hosts.
// An empty host filter means all hosts.
func (c *Coordinator) SummaryByHosts(hostFilter map[string]struct{}) (up int, total int) {
	baseServices, hostHasConsole := c.snapshotServiceSummaryEntries(hostFilter)

	overrideMap := map[string]persistence.WebServiceOverride{}
	manuals := []persistence.WebServiceManual{}
	snapshot, err := c.loadStoreSnapshot()
	if err != nil {
		log.Printf("webservice-coordinator: failed to load service store snapshot for summary: %v", err)
	} else if snapshot != nil {
		overrideMap = snapshot.overrides
		manuals = snapshot.manuals
	}

	seen := make(map[string]struct{}, len(baseServices))
	for _, svc := range baseServices {
		key := serviceOverrideKey(svc.hostAssetID, svc.serviceID)
		seen[key] = struct{}{}

		hidden := svc.hidden
		if svc.serviceKey == labtetherServiceKey {
			switch svc.component {
			case labtetherConsole:
				hidden = false
			case labtetherAPI:
				hidden = hostHasConsole[svc.hostAssetID]
			}
		}
		if override, ok := overrideMap[key]; ok {
			hidden = override.Hidden
		}
		if hidden {
			continue
		}

		total++
		if strings.EqualFold(svc.status, "up") {
			up++
		}
	}

	for _, manual := range manuals {
		hostID := strings.TrimSpace(manual.HostAssetID)
		if !summaryHostAllowed(hostID, hostFilter) {
			continue
		}
		manualID := strings.TrimSpace(manual.ID)
		if manualID == "" {
			continue
		}
		key := serviceOverrideKey(hostID, manualID)
		if _, exists := seen[key]; exists {
			continue
		}

		hidden := false
		if override, ok := overrideMap[key]; ok {
			hidden = override.Hidden
		}
		if hidden {
			continue
		}
		total++
	}

	return up, total
}

func (c *Coordinator) snapshotAllHostServices() []agentmgr.DiscoveredWebService {
	c.mu.RLock()
	defer c.mu.RUnlock()

	all := make([]agentmgr.DiscoveredWebService, 0, 64)
	for _, entry := range c.hosts {
		for _, svc := range entry.services {
			all = append(all, cloneDiscoveredService(svc))
		}
	}
	return all
}

func (c *Coordinator) snapshotHostServices(hostID string) []agentmgr.DiscoveredWebService {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.hosts[hostID]
	if !ok {
		return nil
	}
	out := make([]agentmgr.DiscoveredWebService, 0, len(entry.services))
	for _, svc := range entry.services {
		out = append(out, cloneDiscoveredService(svc))
	}
	return out
}

func (c *Coordinator) snapshotServiceSummaryEntries(
	hostFilter map[string]struct{},
) ([]serviceSummaryEntry, map[string]bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries := make([]serviceSummaryEntry, 0, 64)
	hostHasConsole := make(map[string]bool, len(c.hosts))
	for hostID, entry := range c.hosts {
		if entry == nil {
			continue
		}
		fallbackHostID := strings.TrimSpace(hostID)
		for _, svc := range entry.services {
			serviceHostID := strings.TrimSpace(svc.HostAssetID)
			if serviceHostID == "" {
				serviceHostID = fallbackHostID
			}
			if !summaryHostAllowed(serviceHostID, hostFilter) {
				continue
			}

			serviceKey := strings.TrimSpace(svc.ServiceKey)
			component := ""
			if serviceKey == labtetherServiceKey {
				component = classifyLabTetherComponent(svc)
				if component == labtetherConsole {
					hostHasConsole[serviceHostID] = true
				}
			}

			entries = append(entries, serviceSummaryEntry{
				hostAssetID: serviceHostID,
				serviceID:   strings.TrimSpace(svc.ID),
				status:      strings.TrimSpace(svc.Status),
				serviceKey:  serviceKey,
				component:   component,
				hidden:      isHiddenService(svc),
			})
		}
	}
	return entries, hostHasConsole
}

func summaryHostAllowed(hostID string, hostFilter map[string]struct{}) bool {
	if len(hostFilter) == 0 {
		return true
	}
	if hostID == "" {
		return true // standalone services are not host-scoped
	}
	_, ok := hostFilter[strings.TrimSpace(hostID)]
	return ok
}

func serviceHostMatchesFilter(hostAssetID, hostFilter string) bool {
	filter := strings.TrimSpace(hostFilter)
	if filter == "" {
		return true
	}
	return strings.TrimSpace(hostAssetID) == filter
}

func cloneDiscoveredService(in agentmgr.DiscoveredWebService) agentmgr.DiscoveredWebService {
	out := in
	out.Metadata = cloneMetadata(in.Metadata)
	out.Health = cloneWebServiceHealthSummary(in.Health)
	return out
}

func cloneWebServiceDiscoveryStats(in *agentmgr.WebServiceDiscoveryStats) *agentmgr.WebServiceDiscoveryStats {
	if in == nil {
		return nil
	}
	out := *in
	if len(in.Sources) > 0 {
		out.Sources = make(map[string]agentmgr.WebServiceDiscoverySourceStat, len(in.Sources))
		for key, value := range in.Sources {
			out.Sources[key] = value
		}
	}
	if len(in.FinalSourceCount) > 0 {
		out.FinalSourceCount = make(map[string]int, len(in.FinalSourceCount))
		for key, value := range in.FinalSourceCount {
			out.FinalSourceCount[key] = value
		}
	}
	return &out
}

func cloneMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
