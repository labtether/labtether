package pbs

import (
	"context"
	pbsconnector "github.com/labtether/labtether/internal/connectors/pbs"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
	"net/http"
	"sort"
	"strings"
	"time"
)

// handlePBSTaskRoutes dispatches /pbs/tasks/{node}/{upid}/{action}.
func (d *Deps) HandlePBSTaskRoutes(w http.ResponseWriter, r *http.Request) {
	if denyAssetRestrictedGlobal(w, r, "tasks") {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/pbs/tasks/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "missing task path")
		return
	}
	if strings.HasSuffix(path, "/status") {
		d.HandlePBSTaskStatus(w, r)
		return
	}
	if strings.HasSuffix(path, "/log") {
		d.HandlePBSTaskLog(w, r)
		return
	}
	if strings.HasSuffix(path, "/stop") {
		d.HandlePBSTaskStop(w, r)
		return
	}
	servicehttp.WriteError(w, http.StatusNotFound, "unknown pbs task action")
}

// handlePBSTaskStatus handles GET /pbs/tasks/{node}/{upid}/status.
func (d *Deps) HandlePBSTaskStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	node, upid, ok := ParsePBSTaskPath(r.URL.Path, "status")
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "expected /pbs/tasks/{node}/{upid}/status")
		return
	}
	if node == "" || upid == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "node and upid are required")
		return
	}

	collectorID := strings.TrimSpace(r.URL.Query().Get("collector_id"))
	runtime, err := d.LoadPBSRuntime(collectorID)
	if err != nil {
		writePBSError(w, http.StatusBadGateway, "pbs runtime unavailable", err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	status, err := runtime.Client.GetTaskStatus(ctx, node, upid)
	if err != nil {
		writePBSError(w, http.StatusBadGateway, "failed to fetch pbs task status", err)
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"task": status})
}

// handlePBSTaskLog handles GET /pbs/tasks/{node}/{upid}/log.
func (d *Deps) HandlePBSTaskLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	node, upid, ok := ParsePBSTaskPath(r.URL.Path, "log")
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "expected /pbs/tasks/{node}/{upid}/log")
		return
	}
	if node == "" || upid == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "node and upid are required")
		return
	}

	limit := 200
	if parsed, ok := shared.ParsePositiveInt(r.URL.Query().Get("limit")); ok {
		limit = parsed
	}
	if limit > 2000 {
		limit = 2000
	}
	collectorID := strings.TrimSpace(r.URL.Query().Get("collector_id"))
	runtime, err := d.LoadPBSRuntime(collectorID)
	if err != nil {
		writePBSError(w, http.StatusBadGateway, "pbs runtime unavailable", err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	lines, err := runtime.Client.GetTaskLog(ctx, node, upid, limit)
	if err != nil {
		writePBSError(w, http.StatusBadGateway, "failed to fetch pbs task log", err)
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"lines": lines})
}

// handlePBSTaskStop handles POST /pbs/tasks/{node}/{upid}/stop.
func (d *Deps) HandlePBSTaskStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.RequireAdminAuth(w, r) {
		return
	}
	node, upid, ok := ParsePBSTaskPath(r.URL.Path, "stop")
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "expected /pbs/tasks/{node}/{upid}/stop")
		return
	}
	if node == "" || upid == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "node and upid are required")
		return
	}

	collectorID := strings.TrimSpace(r.URL.Query().Get("collector_id"))
	runtime, err := d.LoadPBSRuntime(collectorID)
	if err != nil {
		writePBSError(w, http.StatusBadGateway, "pbs runtime unavailable", err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := runtime.Client.StopTask(ctx, node, upid); err != nil {
		writePBSError(w, http.StatusBadGateway, "failed to stop pbs task", err)
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func ParsePBSTaskPath(path, action string) (node string, upid string, ok bool) {
	trimmed := strings.TrimPrefix(path, "/pbs/tasks/")
	if trimmed == path || trimmed == "" {
		return "", "", false
	}
	parts := strings.SplitN(trimmed, "/", 3)
	if len(parts) < 3 || strings.TrimSpace(parts[2]) != action {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func FilterAndSortPBSTasks(tasks []pbsconnector.Task, store string, limit int) []pbsconnector.Task {
	filtered := make([]pbsconnector.Task, 0, len(tasks))
	trimmedStore := strings.ToLower(strings.TrimSpace(store))

	for _, task := range tasks {
		if trimmedStore != "" {
			workerID := strings.ToLower(strings.TrimSpace(task.WorkerID))
			upid := strings.ToLower(strings.TrimSpace(task.UPID))
			if !strings.Contains(workerID, trimmedStore) && !strings.Contains(upid, ":"+trimmedStore+":") {
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
		return filtered[:limit]
	}
	return filtered
}
