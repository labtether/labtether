package proxmox

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/servicehttp"
)

// handleProxmoxTaskLog handles GET /proxmox/tasks/{node}/{upid}/log
func (d *Deps) HandleProxmoxTaskLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/proxmox/tasks/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "missing task path")
		return
	}

	// Expect: {node}/{upid}/log
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 3 || parts[2] != "log" {
		servicehttp.WriteError(w, http.StatusNotFound, "expected /proxmox/tasks/{node}/{upid}/log")
		return
	}
	node := strings.TrimSpace(parts[0])
	upid := strings.TrimSpace(parts[1])
	if node == "" || upid == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "node and upid are required")
		return
	}
	collectorID := strings.TrimSpace(r.URL.Query().Get("collector_id"))

	// Find a Proxmox runtime (prefer explicit collector_id when provided).
	runtime, err := d.LoadProxmoxRuntime(collectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "proxmox runtime unavailable: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	log, err := runtime.client.GetTaskLog(ctx, node, upid, 500)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to fetch task log: "+err.Error())
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"log": log})
}

// handleProxmoxTaskStop handles POST /proxmox/tasks/{node}/{upid}/stop
func (d *Deps) HandleProxmoxTaskStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.RequireAdminAuth(w, r) {
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/proxmox/tasks/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "missing task path")
		return
	}

	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 3 || parts[2] != "stop" {
		servicehttp.WriteError(w, http.StatusNotFound, "expected /proxmox/tasks/{node}/{upid}/stop")
		return
	}
	node := strings.TrimSpace(parts[0])
	upid := strings.TrimSpace(parts[1])
	if node == "" || upid == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "node and upid are required")
		return
	}
	collectorID := strings.TrimSpace(r.URL.Query().Get("collector_id"))

	runtime, err := d.LoadProxmoxRuntime(collectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "proxmox runtime unavailable: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := runtime.client.StopTask(ctx, node, upid); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to stop task: "+err.Error())
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func SortProxmoxSnapshots(snapshots []proxmox.Snapshot) []proxmox.Snapshot {
	out := make([]proxmox.Snapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		name := strings.TrimSpace(snapshot.Name)
		if strings.EqualFold(name, "current") {
			continue
		}
		out = append(out, snapshot)
	}
	sort.Slice(out, func(i, j int) bool {
		left := out[i].SnapTime
		right := out[j].SnapTime
		if left == right {
			return strings.ToLower(strings.TrimSpace(out[i].Name)) < strings.ToLower(strings.TrimSpace(out[j].Name))
		}
		return left > right
	})
	return out
}

func FilterAndSortProxmoxTasks(tasks []proxmox.Task, node, vmid string, limit int) []proxmox.Task {
	filtered := make([]proxmox.Task, 0, len(tasks))
	trimmedNode := strings.TrimSpace(node)
	trimmedVMID := strings.TrimSpace(vmid)

	for _, task := range tasks {
		if trimmedNode != "" {
			if taskNode := strings.TrimSpace(task.Node); taskNode != "" && !strings.EqualFold(taskNode, trimmedNode) {
				continue
			}
		}
		if trimmedVMID != "" {
			if !ProxmoxTaskMatchesVMID(task, trimmedVMID) {
				continue
			}
		}
		filtered = append(filtered, task)
	}

	sort.Slice(filtered, func(i, j int) bool {
		left := filtered[i].StartTime
		right := filtered[j].StartTime
		if left == right {
			return strings.ToLower(strings.TrimSpace(filtered[i].UPID)) > strings.ToLower(strings.TrimSpace(filtered[j].UPID))
		}
		return left > right
	})

	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered
}

func ProxmoxTaskMatchesVMID(task proxmox.Task, vmid string) bool {
	trimmed := strings.TrimSpace(vmid)
	if trimmed == "" {
		return true
	}

	id := strings.TrimSpace(task.ID)
	if id != "" {
		if id == trimmed {
			return true
		}
		if parsed, err := strconv.Atoi(trimmed); err == nil && strings.TrimSpace(id) == strconv.Itoa(parsed) {
			return true
		}
		if strings.Contains(id, "/"+trimmed) {
			return true
		}
	}

	upid := strings.TrimSpace(task.UPID)
	return strings.Contains(upid, ":"+trimmed+":")
}

// handleProxmoxClusterStatus handles GET /proxmox/cluster/status
func (d *Deps) HandleProxmoxClusterStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	collectorID := strings.TrimSpace(r.URL.Query().Get("collector_id"))

	runtime, err := d.LoadProxmoxRuntime(collectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "proxmox runtime unavailable: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	entries, err := runtime.client.GetClusterStatus(ctx)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to fetch cluster status: "+err.Error())
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

// handleProxmoxNodeNetwork handles GET /proxmox/nodes/{node}/network
func (d *Deps) HandleProxmoxNodeNetwork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/proxmox/nodes/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 || parts[1] != "network" {
		servicehttp.WriteError(w, http.StatusNotFound, "expected /proxmox/nodes/{node}/network")
		return
	}
	node := strings.TrimSpace(parts[0])
	if node == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "node is required")
		return
	}
	collectorID := strings.TrimSpace(r.URL.Query().Get("collector_id"))

	runtime, err := d.LoadProxmoxRuntime(collectorID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "proxmox runtime unavailable: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	ifaces, err := runtime.client.GetNodeNetwork(ctx, node)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to fetch node network: "+err.Error())
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"interfaces": ifaces})
}

// handleProxmoxNodeRoutes dispatches /proxmox/nodes/{node}/{action}
func (d *Deps) HandleProxmoxNodeRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/proxmox/nodes/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "missing node path")
		return
	}

	// Extract the action segment: /proxmox/nodes/{node}/{action}/...
	// Find the second slash to get the action name.
	slashIdx := strings.Index(path, "/")
	if slashIdx < 0 {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown proxmox node action")
		return
	}
	// action starts after {node}/
	actionPath := path[slashIdx+1:]
	action := actionPath
	if idx := strings.Index(actionPath, "/"); idx >= 0 {
		action = actionPath[:idx]
	}

	switch action {
	case "network":
		d.handleProxmoxNodeNetworkCRUD(w, r)
	case "syslog":
		d.handleProxmoxNodeSyslog(w, r)
	case "replication":
		d.handleProxmoxNodeReplication(w, r)
	case "updates":
		d.handleProxmoxNodeUpdates(w, r)
	case "certificates":
		d.handleProxmoxNodeCertificates(w, r)
	case "storage":
		// /proxmox/nodes/{node}/storage/{storage}/content[/{volid}]
		d.handleProxmoxStorageContent(w, r)
	default:
		servicehttp.WriteError(w, http.StatusNotFound, "unknown proxmox node action")
	}
}

func SelectProxmoxHA(resources []proxmox.HAResource, target ProxmoxSessionTarget) (*proxmox.HAResource, []proxmox.HAResource) {
	related := make([]proxmox.HAResource, 0)
	var expectedSID string
	switch strings.ToLower(strings.TrimSpace(target.Kind)) {
	case "qemu":
		expectedSID = "vm:" + strings.TrimSpace(target.VMID)
	case "lxc":
		expectedSID = "ct:" + strings.TrimSpace(target.VMID)
	}

	var match *proxmox.HAResource
	for i := range resources {
		resource := resources[i]
		resourceSID := strings.TrimSpace(resource.SID)
		if expectedSID != "" {
			if strings.EqualFold(resourceSID, expectedSID) {
				copy := resource
				match = &copy
				related = append(related, resource)
			}
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(target.Kind), "node") {
			continue
		}

		if strings.EqualFold(strings.TrimSpace(resource.Node), strings.TrimSpace(target.Node)) {
			if match == nil {
				copy := resource
				match = &copy
			}
			related = append(related, resource)
		}
	}

	sort.Slice(related, func(i, j int) bool {
		return strings.ToLower(strings.TrimSpace(related[i].SID)) < strings.ToLower(strings.TrimSpace(related[j].SID))
	})
	return match, related
}
