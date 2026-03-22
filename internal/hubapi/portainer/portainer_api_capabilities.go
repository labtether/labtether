package portainer

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/servicehttp"
)

// portainerCapabilities describes the tabs and features available for a given
// Portainer asset.
type PortainerCapabilities struct {
	Tabs      []string `json:"tabs"`
	Kind      string   `json:"kind"` // "container-host", "container", "stack", or other
	CanExec   bool     `json:"can_exec"`
	FetchedAt string   `json:"fetched_at"`
}

var portainerHostTabs = []string{
	"overview",
	"containers",
	"stacks",
	"images",
	"volumes",
	"networks",
}

// handlePortainerCapabilities handles GET /portainer/assets/{id}/capabilities.
func (d *Deps) HandlePortainerCapabilities(w http.ResponseWriter, asset assets.Asset) {
	kind := strings.TrimSpace(strings.ToLower(asset.Type))
	authMethod := d.portainerCapabilityAuthMethod(asset)

	var tabs []string
	if kind == "container-host" {
		tabs = portainerHostTabs
	} else {
		// Containers, stacks, and other Portainer asset types use the metadata
		// view only — no sub-tabs.
		tabs = []string{}
	}

	servicehttp.WriteJSON(w, http.StatusOK, PortainerCapabilities{
		Tabs:      tabs,
		Kind:      kind,
		CanExec:   kind == "container-host" && strings.EqualFold(authMethod, "password"),
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

func (d *Deps) portainerCapabilityAuthMethod(asset assets.Asset) string {
	if d == nil || d.HubCollectorStore == nil {
		return ""
	}

	collectors, err := d.HubCollectorStore.ListHubCollectors(200, true)
	if err != nil {
		return ""
	}

	selected := SelectCollectorForPortainerRuntime(collectors, strings.TrimSpace(asset.Metadata["collector_id"]))
	if selected == nil {
		return ""
	}

	authMethod := strings.ToLower(strings.TrimSpace(shared.CollectorConfigString(selected.Config, "auth_method")))
	if authMethod == "" {
		authMethod = "api_key"
	}
	return authMethod
}
