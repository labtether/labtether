package proxmox

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/connectors/proxmox"
)

func BuildProxmoxStorageInsightEvents(
	tasks []proxmox.Task,
	poolStates []ProxmoxStoragePoolState,
	now time.Time,
	lookback time.Duration,
) []ProxmoxStorageInsightEvent {
	if lookback <= 0 {
		lookback = 24 * time.Hour
	}
	cutoff := now.Add(-lookback).Unix()
	poolIndex := BuildProxmoxStoragePoolIndexByVMID(poolStates)

	events := make([]ProxmoxStorageInsightEvent, 0, len(tasks))
	for _, task := range tasks {
		ts := ProxmoxTaskTimelineTS(task)
		if ts <= 0 || ts < cutoff {
			continue
		}

		vmid := ProxmoxTaskVMID(task)
		pools := poolIndex[vmid]
		if !ProxmoxTaskIsStorageRelevant(task, len(pools) > 0 || vmid > 0) {
			continue
		}

		base := ProxmoxStorageInsightEvent{
			Timestamp:  time.Unix(ts, 0).UTC().Format(time.RFC3339),
			Severity:   ProxmoxStorageTaskSeverity(task),
			Message:    ProxmoxStorageTaskMessage(task, vmid),
			Node:       strings.TrimSpace(task.Node),
			UPID:       strings.TrimSpace(task.UPID),
			TaskType:   strings.TrimSpace(task.Type),
			TaskStatus: strings.TrimSpace(task.Status),
			ExitStatus: strings.TrimSpace(task.ExitStatus),
		}

		if len(pools) == 0 {
			events = append(events, base)
			continue
		}
		for _, poolName := range pools {
			mapped := base
			mapped.Pool = poolName
			events = append(events, mapped)
		}
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].Timestamp != events[j].Timestamp {
			return events[i].Timestamp > events[j].Timestamp
		}
		if events[i].Pool != events[j].Pool {
			return events[i].Pool < events[j].Pool
		}
		return events[i].Message < events[j].Message
	})
	if len(events) > 80 {
		events = events[:80]
	}
	return events
}

func BuildProxmoxStoragePoolIndexByVMID(poolStates []ProxmoxStoragePoolState) map[int][]string {
	result := make(map[int][]string)
	seen := make(map[int]map[string]struct{})

	for _, state := range poolStates {
		poolName := strings.TrimSpace(state.PoolName)
		if poolName == "" {
			continue
		}
		poolKey := NormalizePoolKey(poolName)
		for _, item := range state.Content {
			if item.VMID <= 0 {
				continue
			}
			if _, ok := seen[item.VMID]; !ok {
				seen[item.VMID] = make(map[string]struct{})
			}
			if _, ok := seen[item.VMID][poolKey]; ok {
				continue
			}
			seen[item.VMID][poolKey] = struct{}{}
			result[item.VMID] = append(result[item.VMID], poolName)
		}
	}

	for vmid := range result {
		sort.Strings(result[vmid])
	}
	return result
}

func ProxmoxTaskTimelineTS(task proxmox.Task) int64 {
	if task.EndTime > 0 {
		return int64(task.EndTime)
	}
	if task.StartTime > 0 {
		return int64(task.StartTime)
	}
	return 0
}

func ProxmoxTaskVMID(task proxmox.Task) int {
	if vmid, ok := ParsePositiveInt(task.ID); ok {
		return vmid
	}

	upid := strings.TrimSpace(task.UPID)
	if upid == "" {
		return 0
	}
	parts := strings.Split(upid, ":")
	if len(parts) >= 7 {
		if vmid, ok := ParsePositiveInt(parts[6]); ok {
			return vmid
		}
	}
	return 0
}

func ProxmoxTaskIsStorageRelevant(task proxmox.Task, hasMappedWorkload bool) bool {
	taskType := strings.ToLower(strings.TrimSpace(task.Type))
	if taskType == "" {
		return hasMappedWorkload
	}

	if strings.Contains(taskType, "zfs") ||
		strings.Contains(taskType, "zpool") ||
		strings.Contains(taskType, "storage") ||
		strings.Contains(taskType, "scrub") {
		return true
	}

	if !hasMappedWorkload {
		return false
	}

	return strings.Contains(taskType, "vzdump") ||
		strings.Contains(taskType, "backup") ||
		strings.Contains(taskType, "snapshot") ||
		strings.Contains(taskType, "clone") ||
		strings.Contains(taskType, "migrate") ||
		strings.Contains(taskType, "disk") ||
		strings.Contains(taskType, "move")
}

func ProxmoxStorageTaskSeverity(task proxmox.Task) string {
	status := strings.ToLower(strings.TrimSpace(task.Status))
	exitStatus := strings.ToLower(strings.TrimSpace(task.ExitStatus))

	if status == "running" {
		return "info"
	}
	if status == "error" || (exitStatus != "" && exitStatus != "ok") {
		return "critical"
	}
	return "info"
}

func ProxmoxStorageTaskMessage(task proxmox.Task, vmid int) string {
	taskType := strings.TrimSpace(task.Type)
	if taskType == "" {
		taskType = "task"
	}
	targetSuffix := ""
	if vmid > 0 {
		targetSuffix = " for VM/CT " + strconv.Itoa(vmid)
	}

	status := strings.ToLower(strings.TrimSpace(task.Status))
	exitStatus := strings.TrimSpace(task.ExitStatus)

	switch {
	case status == "running":
		return taskType + " running" + targetSuffix
	case strings.EqualFold(exitStatus, "ok"):
		return taskType + " completed" + targetSuffix
	case exitStatus != "":
		return taskType + " finished with " + exitStatus + targetSuffix
	case status != "":
		return taskType + " status " + status + targetSuffix
	default:
		return taskType + " completed" + targetSuffix
	}
}
