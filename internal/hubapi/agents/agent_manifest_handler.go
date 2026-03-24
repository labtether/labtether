package agents

import (
	"net/http"

	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) HandleAgentManifest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	m := d.AgentCache.Manifest()
	if m == nil {
		http.Error(w, "agent manifest not loaded", http.StatusServiceUnavailable)
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, m)
}

func (d *Deps) HandleAgentCacheRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !d.AgentCache.TryRefresh() {
		http.Error(w, "cache refresh on cooldown, try again later", http.StatusTooManyRequests)
		return
	}

	if err := d.AgentCache.LoadManifest(); err != nil {
		http.Error(w, "failed to reload manifest: "+err.Error(), http.StatusInternalServerError)
		return
	}

	m := d.AgentCache.Manifest()
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"status":  "refreshed",
		"version": m.GoAgentVersion(),
	})
}
