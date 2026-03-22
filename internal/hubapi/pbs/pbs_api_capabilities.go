package pbs

import (
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/servicehttp"
)

// pbsCapabilities describes the tabs and features available for a PBS asset.
type PBSCapabilities struct {
	Tabs      []string `json:"tabs"`
	Kind      string   `json:"kind"`       // "server" or "datastore"
	FetchedAt string   `json:"fetched_at"`
}

var pbsAllTabs = []string{
	"overview",
	"datastores",
	"groups",
	"snapshots",
	"verification",
	"gc",
	"prune-jobs",
	"sync-jobs",
	"tasks",
	"traffic",
	"certificates",
}

// handlePBSCapabilities handles GET /pbs/assets/{id}/capabilities.
func (d *Deps) HandlePBSCapabilities(w http.ResponseWriter, asset assets.Asset) {
	kind := "server"
	if PBSStoreFromAsset(asset) != "" {
		kind = "datastore"
	}

	servicehttp.WriteJSON(w, http.StatusOK, PBSCapabilities{
		Tabs:      pbsAllTabs,
		Kind:      kind,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	})
}
