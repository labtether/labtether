package resources

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

const processRequestTimeout = 10 * time.Second
const (
	DefaultProcessListLimit = 25
	MaxProcessListLimit     = 200
	processActionTimeout    = 30 * time.Second
)

var ValidProcessSignals = map[string]bool{
	"SIGTERM": true,
	"SIGKILL": true,
	"SIGINT":  true,
	"SIGHUP":  true,
}

// processBridge holds the channel for a pending process list request.

// handleProcesses dispatches /processes/{assetId} and /processes/{assetId}/kill requests.
func (d *Deps) HandleProcesses(w http.ResponseWriter, r *http.Request) {
	// Extract path after /processes/
	path := strings.TrimPrefix(r.URL.Path, "/processes/")
	if path == "" || path == r.URL.Path {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset id required")
		return
	}

	parts := strings.SplitN(path, "/", 2)
	assetID := strings.TrimSpace(parts[0])
	if assetID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset id required")
		return
	}

	action := ""
	if len(parts) > 1 {
		action = strings.TrimSpace(parts[1])
	}

	if d.AgentMgr == nil || !d.AgentMgr.IsConnected(assetID) {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent not connected")
		return
	}

	if action == "" {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		d.handleProcessList(w, r, assetID)
		return
	}

	if action != "kill" {
		servicehttp.WriteError(w, http.StatusBadRequest, "unknown process action")
		return
	}
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	d.handleProcessKill(w, r, assetID)
}

func (d *Deps) handleProcessList(w http.ResponseWriter, r *http.Request, assetID string) {
	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent disconnected")
		return
	}

	sortBy := NormalizeProcessSortBy(r.URL.Query().Get("sort"))
	limit := ParseProcessListLimit(r.URL.Query().Get("limit"))

	requestID := generateRequestID()

	bridge := &ProcessBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: assetID,
	}
	d.ProcessBridges.Store(requestID, bridge)
	defer d.ProcessBridges.Delete(requestID)

	data, _ := json.Marshal(agentmgr.ProcessListData{
		RequestID: requestID,
		SortBy:    sortBy,
		Limit:     limit,
	})
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgProcessList,
		ID:   requestID,
		Data: data,
	}); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to send request to agent")
		return
	}

	select {
	case msg := <-bridge.Ch:
		var listed agentmgr.ProcessListedData
		if err := json.Unmarshal(msg.Data, &listed); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
			return
		}
		if listed.Error != "" {
			servicehttp.WriteError(w, http.StatusBadRequest, listed.Error)
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, listed)
	case <-time.After(processRequestTimeout):
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
	}
}

func (d *Deps) handleProcessKill(w http.ResponseWriter, r *http.Request, assetID string) {
	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent disconnected")
		return
	}

	var body struct {
		PID    int    `json:"pid"`
		Signal string `json:"signal"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.PID <= 0 {
		servicehttp.WriteError(w, http.StatusBadRequest, "pid must be greater than zero")
		return
	}

	signal, ok := NormalizeProcessSignal(body.Signal)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid signal: must be one of SIGTERM, SIGKILL, SIGINT, or SIGHUP")
		return
	}

	requestID := generateRequestID()
	bridge := &ProcessBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: assetID,
	}
	d.ProcessBridges.Store(requestID, bridge)
	defer d.ProcessBridges.Delete(requestID)

	data, _ := json.Marshal(agentmgr.ProcessKillData{
		PID:    body.PID,
		Signal: signal,
	})
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgProcessKill,
		ID:   requestID,
		Data: data,
	}); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to send request to agent")
		return
	}

	select {
	case msg := <-bridge.Ch:
		var result agentmgr.ProcessKillResultData
		if err := json.Unmarshal(msg.Data, &result); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
			return
		}
		if result.Error != "" {
			servicehttp.WriteError(w, http.StatusBadRequest, result.Error)
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, result)
	case <-time.After(processActionTimeout):
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
	}
}

func (d *Deps) ProcessAgentProcessListed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.ProcessListedData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid process listed data: %v", err)
		return
	}

	bridgeID := strings.TrimSpace(data.RequestID)
	if bridgeID == "" {
		return
	}

	if raw, ok := d.ProcessBridges.Load(bridgeID); ok {
		bridge, ok := raw.(*ProcessBridge)
		if !ok || bridge == nil {
			return
		}
		if strings.TrimSpace(bridge.ExpectedAssetID) != "" {
			if conn == nil || !strings.EqualFold(strings.TrimSpace(bridge.ExpectedAssetID), strings.TrimSpace(conn.AssetID)) {
				return
			}
		}
		select {
		case bridge.Ch <- msg:
		default:
		}
	}
}

func (d *Deps) ProcessAgentProcessKillResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.ProcessKillResultData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid process kill result data: %v", err)
		return
	}

	bridgeID := strings.TrimSpace(msg.ID)
	if bridgeID == "" {
		return
	}

	if raw, ok := d.ProcessBridges.Load(bridgeID); ok {
		bridge, ok := raw.(*ProcessBridge)
		if !ok || bridge == nil {
			return
		}
		if strings.TrimSpace(bridge.ExpectedAssetID) != "" {
			if conn == nil || !strings.EqualFold(strings.TrimSpace(bridge.ExpectedAssetID), strings.TrimSpace(conn.AssetID)) {
				return
			}
		}
		select {
		case bridge.Ch <- msg:
		default:
		}
	}
}

func (d *Deps) collectProcessListViaCommand(assetID, sortBy string, limit int) (agentmgr.ProcessListedData, error) {
	limit = ClampProcessListLimit(limit)
	sortBy = NormalizeProcessSortBy(sortBy)

	sortColumn := 3 // pcpu
	if sortBy == "memory" {
		sortColumn = 4 // pmem
	}

	requestID := generateRequestID()
	command := fmt.Sprintf(
		"ps -axo user,pid,pcpu,pmem,rss,command | tail -n +2 | sort -k%d,%dnr | head -n %d",
		sortColumn,
		sortColumn,
		limit,
	)

	result := d.ExecuteViaAgent(terminal.CommandJob{
		JobID:       requestID,
		SessionID:   requestID,
		CommandID:   "process.list.fallback",
		Target:      assetID,
		Command:     command,
		Mode:        "agent",
		RequestedAt: time.Now().UTC(),
	})

	if strings.ToLower(strings.TrimSpace(result.Status)) != "succeeded" {
		msg := strings.TrimSpace(result.Output)
		if msg == "" {
			msg = "agent command failed"
		}
		return agentmgr.ProcessListedData{}, fmt.Errorf("%s", msg)
	}

	processes := ParseProcessCommandOutput(result.Output)
	switch sortBy {
	case "memory":
		sort.Slice(processes, func(i, j int) bool { return processes[i].MemPct > processes[j].MemPct })
	default:
		sort.Slice(processes, func(i, j int) bool { return processes[i].CPUPct > processes[j].CPUPct })
	}
	if len(processes) > limit {
		processes = processes[:limit]
	}

	return agentmgr.ProcessListedData{
		RequestID: requestID,
		Processes: processes,
	}, nil
}

func ParseProcessListLimit(raw string) int {
	if raw == "" {
		return DefaultProcessListLimit
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return DefaultProcessListLimit
	}
	return ClampProcessListLimit(n)
}

func ClampProcessListLimit(limit int) int {
	if limit <= 0 {
		return DefaultProcessListLimit
	}
	if limit > MaxProcessListLimit {
		return MaxProcessListLimit
	}
	return limit
}

func NormalizeProcessSortBy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "memory":
		return "memory"
	default:
		return "cpu"
	}
}

func NormalizeProcessSignal(raw string) (string, bool) {
	trimmed := strings.ToUpper(strings.TrimSpace(raw))
	if trimmed == "" {
		return "SIGTERM", true
	}
	if !strings.HasPrefix(trimmed, "SIG") {
		trimmed = "SIG" + trimmed
	}
	return trimmed, ValidProcessSignals[trimmed]
}

func ParseProcessCommandOutput(output string) []agentmgr.ProcessInfo {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return nil
	}

	lines := strings.Split(trimmed, "\n")
	processes := make([]agentmgr.ProcessInfo, 0, len(lines))
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		pid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		cpuPct, _ := strconv.ParseFloat(fields[2], 64)
		memPct, _ := strconv.ParseFloat(fields[3], 64)
		memRSS, _ := strconv.ParseInt(fields[4], 10, 64) // RSS in KB
		command := strings.Join(fields[5:], " ")
		name := command
		if parts := strings.Fields(command); len(parts) > 0 {
			exe := parts[0]
			if idx := strings.LastIndexByte(exe, '/'); idx >= 0 {
				exe = exe[idx+1:]
			}
			name = exe
		}

		processes = append(processes, agentmgr.ProcessInfo{
			PID:     pid,
			Name:    name,
			User:    fields[0],
			CPUPct:  cpuPct,
			MemPct:  memPct,
			MemRSS:  memRSS,
			Command: command,
		})
	}
	return processes
}
