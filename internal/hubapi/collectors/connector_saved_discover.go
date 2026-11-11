package collectors

import (
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/modelmap"
	"github.com/labtether/labtether/internal/servicehttp"
)

// writeConnectorDiscover answers GET /connectors/{id}/discover: the connector
// set by environment variables when it finds something, else what the
// connections saved in the console found.
func (d *Deps) writeConnectorDiscover(w http.ResponseWriter, r *http.Request, connectorID, resolvedConnectorID string, connector connectorsdk.Connector) {
	discovered, err := connector.Discover(r.Context())
	fromSaved := false
	if err != nil || len(discovered) == 0 {
		if saved, found := d.savedConnectionAssets(resolvedConnectorID); found {
			discovered, err, fromSaved = saved, nil, true
		}
	}
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "discover failed")
		return
	}
	canonicalAssets := modelmap.CanonicalizeConnectorAssets(connectorID, discovered)
	if !fromSaved && d.PersistCanonicalConnectorSnapshot != nil {
		d.PersistCanonicalConnectorSnapshot(resolvedConnectorID, "", connector.DisplayName(), "", connector, canonicalAssets)
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"assets":          canonicalAssets,
		"relationships":   modelmap.SynthesizeResourceRelationships(connectorID, canonicalAssets),
		"capability_sets": modelmap.SynthesizeCapabilitySets(connector, canonicalAssets),
	})
}

// savedConnectionAssets returns the devices that this connector's enabled
// connections saved in the console found at their last sync. Discover read
// only the connector set by environment variables, so a Hub set up in the
// console answered an empty list. It calls nothing outside the Hub.
func (d *Deps) savedConnectionAssets(connectorID string) ([]connectorsdk.Asset, bool) {
	_, connections, found := d.SavedConnectionsHealth(connectorID)
	if !found || d.AssetStore == nil {
		return nil, false
	}
	owners := make(map[string]struct{}, len(connections))
	for _, connection := range connections {
		if connection.Enabled {
			owners[connection.CollectorID] = struct{}{}
		}
	}
	list, err := d.AssetStore.ListAssets()
	if err != nil {
		return nil, false
	}
	out := make([]connectorsdk.Asset, 0)
	for _, stored := range list {
		if strings.EqualFold(strings.TrimSpace(stored.Source), "agent") {
			continue
		}
		if _, ok := owners[strings.TrimSpace(stored.Metadata["collector_id"])]; !ok {
			continue
		}
		metadata := make(map[string]string, len(stored.Metadata))
		for key, value := range stored.Metadata {
			metadata[key] = value
		}
		out = append(out, connectorsdk.Asset{
			ID: stored.ID, Type: stored.Type, Name: stored.Name, Source: stored.Source, Metadata: metadata,
		})
	}
	return out, true
}
