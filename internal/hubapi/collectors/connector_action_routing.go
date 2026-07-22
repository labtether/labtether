package collectors

import (
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/hubcollector"
)

// ValidateSingletonConnectorActionRoute prevents the legacy singleton
// connector registry from executing against an arbitrary runtime when several
// independently configured collectors of the same type are active.
func (d *Deps) ValidateSingletonConnectorActionRoute(connectorID, targetID string) error {
	collectorType := collectorTypeForActionRoute(connectorID)
	if collectorType == "" || collectorType == hubcollector.CollectorTypeProxmox {
		return nil
	}
	_, targetIsScoped := assetid.CollectorScopeFromAssetID(targetID)
	if d.HubCollectorStore == nil {
		if targetIsScoped {
			return fmt.Errorf("cannot verify collector-aware action routing for %s", collectorType)
		}
		return nil
	}
	collectors, err := d.HubCollectorStore.ListHubCollectors(200, true)
	if err != nil {
		if targetIsScoped {
			return fmt.Errorf("cannot verify collector-aware action routing for %s", collectorType)
		}
		return nil
	}
	count := 0
	for _, collector := range collectors {
		if hubcollector.NormalizeCollectorType(collector.CollectorType) == collectorType {
			count++
		}
	}
	if count > 1 {
		return fmt.Errorf("multiple active %s collectors are configured; singleton connector action dispatch is disabled", collectorType)
	}
	return nil
}

func collectorTypeForActionRoute(connectorID string) string {
	switch strings.ToLower(strings.TrimSpace(connectorID)) {
	case "home-assistant", hubcollector.CollectorTypeHomeAssistant:
		return hubcollector.CollectorTypeHomeAssistant
	case hubcollector.CollectorTypePortainer:
		return hubcollector.CollectorTypePortainer
	case hubcollector.CollectorTypePBS:
		return hubcollector.CollectorTypePBS
	case hubcollector.CollectorTypeTrueNAS:
		return hubcollector.CollectorTypeTrueNAS
	case hubcollector.CollectorTypeProxmox:
		return hubcollector.CollectorTypeProxmox
	case hubcollector.CollectorTypeDocker:
		return hubcollector.CollectorTypeDocker
	default:
		return ""
	}
}
