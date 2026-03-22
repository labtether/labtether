package collectors

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/logs"
)

func (d *Deps) AppendConnectorLogEvent(assetID, source, level, message string, fields map[string]string, at time.Time) {
	d.AppendConnectorLogEventWithID("", assetID, source, level, message, fields, at)
}

func (d *Deps) AppendConnectorLogEventWithID(eventID, assetID, source, level, message string, fields map[string]string, at time.Time) {
	if d.LogStore == nil {
		return
	}

	trimmedMessage := strings.TrimSpace(message)
	if trimmedMessage == "" {
		return
	}
	trimmedSource := strings.TrimSpace(source)
	if trimmedSource == "" {
		trimmedSource = "hub-collector"
	}
	trimmedLevel := normalizeCollectorLogLevel(level)

	if at.IsZero() {
		at = time.Now().UTC()
	}
	if fields != nil {
		fields = cloneStringMap(fields)
	}

	_ = d.LogStore.AppendEvent(logs.Event{
		ID:        strings.TrimSpace(eventID),
		AssetID:   strings.TrimSpace(assetID),
		Source:    trimmedSource,
		Level:     trimmedLevel,
		Message:   trimmedMessage,
		Fields:    fields,
		Timestamp: at.UTC(),
	})
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func normalizeCollectorLogLevel(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(lower, "crit"), strings.Contains(lower, "err"), strings.Contains(lower, "fail"):
		return "error"
	case strings.Contains(lower, "warn"):
		return "warn"
	default:
		return "info"
	}
}

func StableConnectorLogID(prefix, key string) string {
	trimmedPrefix := strings.TrimSpace(prefix)
	trimmedKey := strings.TrimSpace(key)
	if trimmedPrefix == "" || trimmedKey == "" {
		return ""
	}

	encoded := base64.RawURLEncoding.EncodeToString([]byte(trimmedKey))
	if len(encoded) > 120 {
		encoded = encoded[:120]
	}
	return trimmedPrefix + "_" + encoded
}

func (d *Deps) ingestProxmoxTaskLogs(ctx context.Context, client *proxmox.Client, fallbackAssetID string) (int, error) {
	tasks, err := client.ListClusterTasks(ctx, "", "", 30)
	if err != nil {
		return 0, err
	}

	ingested := 0
	for _, task := range tasks {
		upid := strings.TrimSpace(task.UPID)
		if upid == "" {
			continue
		}

		d.AppendConnectorLogEventWithID(
			StableConnectorLogID("log_proxmox_task", upid),
			ProxmoxTaskAssetID(task, fallbackAssetID),
			"proxmox",
			ProxmoxTaskLevel(task),
			proxmoxTaskMessage(task),
			map[string]string{
				"upid":        upid,
				"node":        strings.TrimSpace(task.Node),
				"task_type":   strings.TrimSpace(task.Type),
				"task_target": strings.TrimSpace(task.ID),
				"task_status": strings.TrimSpace(task.Status),
				"exit_status": strings.TrimSpace(task.ExitStatus),
				"user":        strings.TrimSpace(task.User),
			},
			proxmoxTaskTimestamp(task),
		)
		ingested++
	}

	return ingested, nil
}

func ProxmoxTaskAssetID(task proxmox.Task, fallbackAssetID string) string {
	target := strings.TrimSpace(task.ID)
	if target != "" {
		parts := strings.Split(target, "/")
		if len(parts) >= 2 {
			kind := strings.ToLower(strings.TrimSpace(parts[0]))
			idPart := strings.TrimSpace(parts[1])
			switch kind {
			case "qemu":
				if idPart != "" {
					return "proxmox-vm-" + idPart
				}
			case "lxc":
				if idPart != "" {
					return "proxmox-ct-" + idPart
				}
			case "node":
				if idPart != "" {
					return "proxmox-node-" + NormalizeAssetKey(idPart)
				}
			}
		}
	}

	if node := strings.TrimSpace(task.Node); node != "" {
		return "proxmox-node-" + NormalizeAssetKey(node)
	}
	return strings.TrimSpace(fallbackAssetID)
}

func proxmoxTaskMessage(task proxmox.Task) string {
	action := strings.TrimSpace(task.Type)
	if action == "" {
		action = "task"
	}
	target := strings.TrimSpace(task.ID)
	if target == "" {
		target = strings.TrimSpace(task.UPID)
	}
	state := strings.TrimSpace(task.ExitStatus)
	if state == "" {
		state = strings.TrimSpace(task.Status)
	}
	if state == "" {
		state = "unknown"
	}
	node := strings.TrimSpace(task.Node)
	if node == "" {
		return fmt.Sprintf("proxmox %s %s: %s", action, target, state)
	}
	return fmt.Sprintf("proxmox %s %s on %s: %s", action, target, node, state)
}

func ProxmoxTaskLevel(task proxmox.Task) string {
	exitStatus := strings.ToLower(strings.TrimSpace(task.ExitStatus))
	status := strings.ToLower(strings.TrimSpace(task.Status))
	switch {
	case strings.Contains(exitStatus, "error"), strings.Contains(exitStatus, "fail"),
		strings.Contains(status, "error"), strings.Contains(status, "fail"):
		return "error"
	case strings.Contains(exitStatus, "warn"), strings.Contains(status, "warn"):
		return "warn"
	default:
		return "info"
	}
}

func proxmoxTaskTimestamp(task proxmox.Task) time.Time {
	if task.EndTime > 0 {
		return time.Unix(int64(task.EndTime), 0).UTC()
	}
	if task.StartTime > 0 {
		return time.Unix(int64(task.StartTime), 0).UTC()
	}
	return time.Now().UTC()
}

func (d *Deps) IngestTrueNASAlertLogs(ctx context.Context, client *truenas.Client, fallbackAssetID string) (int, error) {
	alerts := make([]map[string]any, 0, 32)
	if err := client.Call(ctx, "alert.list", nil, &alerts); err != nil {
		if truenas.IsMethodNotFound(err) {
			return 0, nil
		}
		return 0, err
	}

	ingested := 0
	for _, alert := range alerts {
		key := strings.TrimSpace(CollectorAnyString(alert["uuid"]))
		if key == "" {
			key = strings.TrimSpace(CollectorAnyString(alert["id"]))
		}
		if key == "" {
			key = strings.TrimSpace(CollectorAnyString(alert["formatted"])) + "|" + strings.TrimSpace(CollectorAnyString(alert["datetime"]))
		}
		if key == "|" {
			continue
		}

		assetID := strings.TrimSpace(fallbackAssetID)
		if hostname := strings.TrimSpace(CollectorAnyString(alert["hostname"])); hostname != "" {
			assetID = "truenas-host-" + NormalizeAssetKey(hostname)
		}

		fields := map[string]string{
			"alert_class":  strings.TrimSpace(CollectorAnyString(alert["klass"])),
			"alert_source": strings.TrimSpace(CollectorAnyString(alert["source"])),
		}
		if node := strings.TrimSpace(CollectorAnyString(alert["node"])); node != "" {
			fields["node"] = node
		}
		if hostname := strings.TrimSpace(CollectorAnyString(alert["hostname"])); hostname != "" {
			fields["hostname"] = hostname
		}

		d.AppendConnectorLogEventWithID(
			StableConnectorLogID("log_truenas_alert", key),
			assetID,
			"truenas",
			normalizeCollectorLogLevel(CollectorAnyString(alert["level"])),
			TrueNASAlertMessage(alert),
			fields,
			CollectorAnyTime(alert["datetime"]),
		)
		ingested++
	}

	return ingested, nil
}

func TrueNASAlertMessage(alert map[string]any) string {
	if formatted := strings.TrimSpace(CollectorAnyString(alert["formatted"])); formatted != "" {
		return formatted
	}
	if text := strings.TrimSpace(CollectorAnyString(alert["text"])); text != "" {
		return text
	}

	klass := strings.TrimSpace(CollectorAnyString(alert["klass"]))
	if klass == "" {
		return "truenas alert"
	}
	return "truenas alert: " + klass
}

func CollectorAnyString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	case float64:
		return strings.TrimSpace(strconv.FormatFloat(typed, 'f', -1, 64))
	case int:
		return strings.TrimSpace(strconv.Itoa(typed))
	case int64:
		return strings.TrimSpace(strconv.FormatInt(typed, 10))
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func CollectorAnyTime(value any) time.Time {
	switch typed := value.(type) {
	case time.Time:
		return typed.UTC()
	case float64:
		if typed > 0 {
			return time.Unix(int64(typed), 0).UTC()
		}
	case int64:
		if typed > 0 {
			return time.Unix(typed, 0).UTC()
		}
	case int:
		if typed > 0 {
			return time.Unix(int64(typed), 0).UTC()
		}
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			break
		}
		if parsed, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
			return parsed.UTC()
		}
		if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
			return parsed.UTC()
		}
		if unix, err := strconv.ParseFloat(trimmed, 64); err == nil && unix > 0 {
			return time.Unix(int64(unix), 0).UTC()
		}
	}
	return time.Now().UTC()
}
