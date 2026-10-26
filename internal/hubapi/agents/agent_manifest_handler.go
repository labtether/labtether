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

	// This endpoint re-reads agent-manifest.json from the runtime cache and
	// the image; it does not download anything. Say so, and report what was
	// loaded, rather than claiming a remote refresh.
	m := d.AgentCache.Manifest()
	versions := make(map[string]string, len(m.Agents))
	for name, entry := range m.Agents {
		versions[name] = entry.Version
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"status":       "reloaded",
		"source":       "disk",
		"message":      "agent manifest reloaded from disk; no remote download was performed",
		"generated_at": m.GeneratedAt,
		"version":      m.GoAgentVersion(),
		"versions":     versions,
	})
}
