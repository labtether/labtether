package collectors

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/servicehttp"
)

// SavedConnection is one connection saved in the console (a hub collector),
// as connector health reports it.
type SavedConnection struct {
	CollectorID     string     `json:"collector_id"`
	AssetID         string     `json:"asset_id"`
	Enabled         bool       `json:"enabled"`
	Status          string     `json:"status"`
	LastError       string     `json:"last_error,omitempty"`
	LastCollectedAt *time.Time `json:"last_collected_at,omitempty"`
}

// SavedConnectionsHealth summarises the connections of one connector that were
// saved in the console, from each one's last collection. It calls nothing
// outside the Hub. found is false when the connector has no saved connection;
// then only the Hub's environment settings can configure it.
func (d *Deps) SavedConnectionsHealth(connectorID string) (health connectorsdk.Health, connections []SavedConnection, found bool) {
	if d.HubCollectorStore == nil {
		return connectorsdk.Health{}, nil, false
	}
	collectorType := normalizeConnectorID(connectorID)
	if collectorType == "home-assistant" {
		collectorType = "homeassistant"
	}
	saved, err := d.HubCollectorStore.ListHubCollectors(500, false)
	if err != nil {
		return connectorsdk.Health{Status: "failed", Message: "saved connections could not be read"}, nil, true
	}
	counts := map[string]int{}
	for _, collector := range saved {
		if collector.CollectorType != collectorType {
			continue
		}
		entry := SavedConnection{
			CollectorID:     collector.ID,
			AssetID:         collector.AssetID,
			Enabled:         collector.Enabled,
			Status:          savedConnectionStatus(collector.Enabled, collector.LastStatus, collector.LastCollectedAt),
			LastError:       strings.TrimSpace(collector.LastError),
			LastCollectedAt: collector.LastCollectedAt,
		}
		counts[entry.Status]++
		connections = append(connections, entry)
	}
	if len(connections) == 0 {
		return connectorsdk.Health{}, nil, false
	}
	return summariseSavedConnections(counts, len(connections)), connections, true
}

func savedConnectionStatus(enabled bool, lastStatus string, collectedAt *time.Time) string {
	switch {
	case !enabled:
		return "disabled"
	case collectedAt == nil:
		return "pending"
	}
	switch strings.ToLower(strings.TrimSpace(lastStatus)) {
	case "ok":
		return "ok"
	case "partial":
		return "partial"
	default:
		return "failed"
	}
}

func summariseSavedConnections(counts map[string]int, total int) connectorsdk.Health {
	plural := func(n int) string {
		if n == 1 {
			return "1 saved connection"
		}
		return fmt.Sprintf("%d saved connections", n)
	}
	enabled := total - counts["disabled"]
	switch {
	case enabled == 0:
		return connectorsdk.Health{Status: "disabled", Message: plural(total) + ", all turned off"}
	case counts["ok"] == enabled:
		return connectorsdk.Health{Status: "ok", Message: plural(total) + ", working at the last check"}
	case counts["pending"] == enabled:
		return connectorsdk.Health{Status: "pending", Message: plural(total) + ", not checked yet"}
	case counts["ok"] == 0 && counts["partial"] == 0 && counts["pending"] == 0:
		return connectorsdk.Health{Status: "failed", Message: plural(total) + ", none working at the last check"}
	default:
		return connectorsdk.Health{Status: "partial", Message: fmt.Sprintf("%s, %d working at the last check", plural(total), counts["ok"])}
	}
}

// writeConnectorHealth answers GET /connectors/{id}/health: the connections
// saved in the console when there are any, else a test of the connector the
// Hub's environment settings configure.
func (d *Deps) writeConnectorHealth(w http.ResponseWriter, r *http.Request, connectorID string, connector connectorsdk.Connector) {
	if health, connections, found := d.SavedConnectionsHealth(connectorID); found {
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"status": health.Status, "message": health.Message, "connections": connections,
		})
		return
	}
	health, err := connector.TestConnection(r.Context())
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "health check failed")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, health)
}
