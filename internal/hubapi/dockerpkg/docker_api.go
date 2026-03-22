package dockerpkg

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/connectors/docker"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/servicehttp"
)

// DockerHostSummary is the API representation of a Docker host.
type DockerHostSummary struct {
	AgentID        string    `json:"agent_id"`
	NormalizedID   string    `json:"normalized_id"`
	EngineVersion  string    `json:"engine_version"`
	EngineOS       string    `json:"engine_os"`
	EngineArch     string    `json:"engine_arch"`
	Hostname       string    `json:"hostname,omitempty"`
	ContainerCount int       `json:"container_count"`
	StackCount     int       `json:"stack_count"`
	ImageCount     int       `json:"image_count"`
	LastSeen       time.Time `json:"last_seen"`
}

// HandleDockerHosts handles GET /api/v1/docker/hosts — list all Docker hosts.
func (d *Deps) HandleDockerHosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.DockerCoordinator == nil {
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"hosts": []any{}})
		return
	}

	hosts := d.DockerCoordinator.ListHosts()
	summaries := make([]DockerHostSummary, 0, len(hosts))
	for _, h := range hosts {
		norm := NormalizeDockerHostLookupID(h.AgentID)
		summaries = append(summaries, DockerHostSummary{
			AgentID:        h.AgentID,
			NormalizedID:   norm,
			EngineVersion:  h.Engine.Version,
			EngineOS:       h.Engine.OS,
			EngineArch:     h.Engine.Arch,
			Hostname:       h.Engine.Hostname,
			ContainerCount: len(h.Containers),
			StackCount:     len(h.ComposeStacks),
			ImageCount:     len(h.Images),
			LastSeen:       h.LastSeen,
		})
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"hosts": summaries})
}

// HandleDockerHostActions handles /api/v1/docker/hosts/{id}[/sub-resource].
//
// Routes:
//
//	GET  /api/v1/docker/hosts/{id}             — single host detail
//	GET  /api/v1/docker/hosts/{id}/containers  — containers on host
//	GET  /api/v1/docker/hosts/{id}/images      — images on host
//	GET  /api/v1/docker/hosts/{id}/stacks      — stacks on host
func (d *Deps) HandleDockerHostActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/docker/hosts/")
	if path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "missing host id")
		return
	}

	parts := strings.SplitN(path, "/", 2)
	hostID := strings.TrimSpace(parts[0])
	if hostID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "missing host id")
		return
	}

	if d.DockerCoordinator == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "docker coordinator not available")
		return
	}

	host, ok := d.resolveDockerHostFromRoute(hostID)
	if !ok || host == nil {
		servicehttp.WriteError(w, http.StatusNotFound, "docker host not found")
		return
	}

	if len(parts) == 1 {
		// GET /api/v1/docker/hosts/{id} — full host detail.
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		enriched := DockerHostWithContainerStats(host)
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"host": enriched})
		return
	}

	switch parts[1] {
	case "containers":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"containers": DockerContainersWithStats(host.Containers, host.Stats),
		})
	case "images":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"images": host.Images})
	case "stacks":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"stacks": host.ComposeStacks})
	case "action":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var body struct {
			Action   string            `json:"action"`
			TargetID string            `json:"target_id,omitempty"`
			Params   map[string]string `json:"params,omitempty"`
		}
		if err := d.DecodeJSONBody(w, r, &body); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid action payload")
			return
		}
		actionID := strings.TrimSpace(body.Action)
		if actionID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "action is required")
			return
		}
		targetID := strings.TrimSpace(body.TargetID)
		if targetID == "" {
			targetID = hostID
		}
		result, err := d.ExecuteDockerAction(r.Context(), actionID, connectorsdk.ActionRequest{
			TargetID: targetID,
			Params:   body.Params,
		})
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "action execution failed: "+d.SanitizeUpstreamError(err.Error()))
			return
		}
		status := http.StatusOK
		if strings.EqualFold(result.Status, "failed") {
			status = http.StatusBadRequest
		}
		servicehttp.WriteJSON(w, status, map[string]any{"result": result})
	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown docker host sub-resource")
	}
}

func (d *Deps) resolveDockerHostFromRoute(hostID string) (*docker.DockerHost, bool) {
	normalizedRouteID := NormalizeDockerHostLookupID(hostID)
	if normalizedRouteID == "" || d.DockerCoordinator == nil {
		return nil, false
	}

	lookupIDs := []string{normalizedRouteID}
	if strings.HasPrefix(normalizedRouteID, "docker-host-") {
		lookupIDs = append(lookupIDs, strings.TrimPrefix(normalizedRouteID, "docker-host-"))
	}

	seen := make(map[string]struct{}, len(lookupIDs))
	for _, lookupID := range lookupIDs {
		lookupID = strings.TrimSpace(lookupID)
		if lookupID == "" {
			continue
		}
		if _, exists := seen[lookupID]; exists {
			continue
		}
		seen[lookupID] = struct{}{}
		if host, ok := d.DockerCoordinator.GetHostByNormalizedID(lookupID); ok {
			return host, true
		}
	}

	return nil, false
}

// NormalizeDockerHostLookupID normalizes a Docker host ID for lookup.
func NormalizeDockerHostLookupID(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, " ", "-")
	normalized = strings.ReplaceAll(normalized, ".", "-")
	return normalized
}

// DockerHostWithContainerStats enriches a Docker host with container stats.
func DockerHostWithContainerStats(host *docker.DockerHost) *docker.DockerHost {
	if host == nil {
		return nil
	}
	copyHost := *host
	copyHost.Containers = DockerContainersWithStats(host.Containers, host.Stats)
	return &copyHost
}

// DockerContainersWithStats enriches containers with stats data.
func DockerContainersWithStats(containers []docker.ContainerState, stats map[string]docker.ContainerStats) []docker.ContainerState {
	if len(containers) == 0 {
		return []docker.ContainerState{}
	}
	result := make([]docker.ContainerState, len(containers))
	copy(result, containers)
	for i := range result {
		stat, ok := stats[result[i].ID]
		if !ok {
			continue
		}
		cpu := stat.CPUPercent
		memPct := stat.MemoryPercent
		memBytes := stat.MemoryBytes
		memLimit := stat.MemoryLimit
		result[i].CPUPercent = &cpu
		result[i].MemoryPercent = &memPct
		result[i].MemoryBytes = &memBytes
		result[i].MemoryLimit = &memLimit
	}
	return result
}

// HandleDockerContainerActions handles /api/v1/docker/containers/{id}[/sub].
//
// Routes:
//
//	GET  /api/v1/docker/containers/{id}         — container detail
//	GET  /api/v1/docker/containers/{id}/stats   — container stats
//	POST /api/v1/docker/containers/{id}/action  — execute container action
func (d *Deps) HandleDockerContainerActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/docker/containers/")
	if path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "missing container id")
		return
	}

	parts := strings.SplitN(path, "/", 2)
	containerAssetID := strings.TrimSpace(parts[0])
	if containerAssetID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "missing container id")
		return
	}

	if d.DockerCoordinator == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "docker coordinator not available")
		return
	}

	host, ct, ok := d.DockerCoordinator.FindContainer(containerAssetID)
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "container not found")
		return
	}

	// Root: GET /api/v1/docker/containers/{id}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"container": ct,
			"agent_id":  host.AgentID,
		})
		return
	}

	switch parts[1] {
	case "stats":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		stats, hasStats := d.DockerCoordinator.GetContainerStats(host.AgentID, ct.ID)
		if !hasStats {
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"stats": nil})
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"stats": stats})
	case "logs":
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		params := map[string]string{}
		if tail := strings.TrimSpace(r.URL.Query().Get("tail")); tail != "" {
			params["tail"] = tail
		}
		if timestamps := strings.TrimSpace(r.URL.Query().Get("timestamps")); timestamps != "" {
			params["timestamps"] = timestamps
		}
		result, err := d.ExecuteDockerAction(r.Context(), "container.logs", connectorsdk.ActionRequest{
			TargetID: containerAssetID,
			Params:   params,
		})
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "failed to load container logs: "+d.SanitizeUpstreamError(err.Error()))
			return
		}
		if strings.EqualFold(result.Status, "failed") {
			servicehttp.WriteError(w, http.StatusBadRequest, result.Message)
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"logs": result.Output})

	case "action":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var body struct {
			Action string            `json:"action"`
			Params map[string]string `json:"params,omitempty"`
		}
		if err := d.DecodeJSONBody(w, r, &body); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid action payload")
			return
		}
		actionID := strings.TrimSpace(body.Action)
		if actionID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "action is required")
			return
		}
		req := connectorsdk.ActionRequest{
			TargetID: containerAssetID,
			Params:   body.Params,
		}
		result, err := d.ExecuteDockerAction(r.Context(), actionID, req)
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "action execution failed: "+d.SanitizeUpstreamError(err.Error()))
			return
		}
		status := http.StatusOK
		if strings.EqualFold(result.Status, "failed") {
			status = http.StatusBadRequest
		}
		servicehttp.WriteJSON(w, status, map[string]any{"result": result})

	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown container sub-resource")
	}
}

// HandleDockerStackActions handles /api/v1/docker/stacks/{id}[/sub].
//
// Routes:
//
//	POST /api/v1/docker/stacks/{id}/action  — execute stack action
func (d *Deps) HandleDockerStackActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/docker/stacks/")
	if path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "missing stack id")
		return
	}

	parts := strings.SplitN(path, "/", 2)
	stackAssetID := strings.TrimSpace(parts[0])
	if stackAssetID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "missing stack id")
		return
	}

	if d.DockerCoordinator == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "docker coordinator not available")
		return
	}

	if len(parts) == 1 || parts[1] == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "missing stack sub-resource")
		return
	}

	switch parts[1] {
	case "action":
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var body struct {
			Action string            `json:"action"`
			Params map[string]string `json:"params,omitempty"`
		}
		if err := d.DecodeJSONBody(w, r, &body); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid action payload")
			return
		}
		actionID := strings.TrimSpace(body.Action)
		if actionID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "action is required")
			return
		}
		req := connectorsdk.ActionRequest{
			TargetID: stackAssetID,
			Params:   body.Params,
		}
		result, err := d.ExecuteDockerAction(r.Context(), actionID, req)
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, "action execution failed: "+d.SanitizeUpstreamError(err.Error()))
			return
		}
		status := http.StatusOK
		if strings.EqualFold(result.Status, "failed") {
			status = http.StatusBadRequest
		}
		servicehttp.WriteJSON(w, status, map[string]any{"result": result})

	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown stack sub-resource")
	}
}
