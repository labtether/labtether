package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/hubapi/shared"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/platforms"
	"github.com/labtether/labtether/internal/telemetry"
)

func (d *Deps) ProcessAgentHeartbeat(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.HeartbeatData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid heartbeat data from %s: %v", conn.AssetID, err)
		return
	}

	// Resolve platform the same way as the HTTP handler.
	resolvedPlatform := platforms.Resolve(
		data.Platform,
		data.Metadata["platform"],
		data.Metadata["os"],
		data.Metadata["os_name"],
		data.Metadata["os_pretty_name"],
	)
	if resolvedPlatform != "" {
		if data.Metadata == nil {
			data.Metadata = map[string]string{}
		}
		data.Metadata["platform"] = resolvedPlatform
	}

	req := assets.HeartbeatRequest{
		AssetID:  conn.AssetID,
		Type:     data.Type,
		Name:     data.Name,
		Source:   data.Source,
		GroupID:  data.GroupID,
		Status:   data.Status,
		Platform: resolvedPlatform,
		Metadata: data.Metadata,
	}
	webrtcAvailable := strings.EqualFold(strings.TrimSpace(data.Metadata["webrtc_available"]), "true")
	if !webrtcAvailable {
		for _, capability := range data.Capabilities {
			if strings.EqualFold(strings.TrimSpace(capability), "webrtc") {
				webrtcAvailable = true
				break
			}
		}
	}
	conn.SetMeta("webrtc_available", fmt.Sprintf("%v", webrtcAvailable))
	conn.SetMeta("webrtc_video_encoders", strings.TrimSpace(data.Metadata["webrtc_video_encoders"]))
	conn.SetMeta("webrtc_audio_sources", strings.TrimSpace(data.Metadata["webrtc_audio_sources"]))
	conn.SetMeta("webrtc_unavailable_reason", strings.TrimSpace(data.Metadata["webrtc_unavailable_reason"]))
	conn.SetMeta("desktop_session_type", strings.TrimSpace(data.Metadata["desktop_session_type"]))
	conn.SetMeta("desktop_backend", strings.TrimSpace(data.Metadata["desktop_backend"]))
	conn.SetMeta("desktop_capture_backend", strings.TrimSpace(data.Metadata["desktop_capture_backend"]))
	conn.SetMeta("desktop_vnc_real_desktop_supported", strings.TrimSpace(data.Metadata["desktop_vnc_real_desktop_supported"]))
	conn.SetMeta("desktop_webrtc_real_desktop_supported", strings.TrimSpace(data.Metadata["desktop_webrtc_real_desktop_supported"]))
	if reportedVersion := strings.TrimSpace(data.Metadata["agent_version"]); reportedVersion != "" {
		conn.SetMeta("agent_version", reportedVersion)
	}

	if _, err := d.ProcessHeartbeatRequest(req); err != nil {
		log.Printf("agentws: heartbeat processing failed for %s: %v", conn.AssetID, err)
	}
	d.AutoProvisionDockerCollectorIfNeeded(conn.AssetID, data.Connectors)

	// Update presence heartbeat timestamp.
	if d.PresenceStore != nil {
		sessionID := strings.TrimSpace(conn.Meta("presence.session_id"))
		if sessionID != "" {
			_, _ = d.PresenceStore.UpdateHeartbeatForSession(conn.AssetID, sessionID, time.Now().UTC())
		} else {
			_ = d.PresenceStore.UpdateHeartbeat(conn.AssetID, time.Now().UTC())
		}
	}

	d.broadcastEvent("heartbeat.update", map[string]any{"asset_id": conn.AssetID})

	// If the agent reported its version and we know the latest version, push an
	// update request when the agent is outdated. The push is fire-and-forget —
	// the agent decides locally whether auto-update is enabled.
	if agentVersion := strings.TrimSpace(conn.Meta("agent_version")); agentVersion != "" {
		if d.AgentCache != nil {
			if manifest := d.AgentCache.Manifest(); manifest != nil {
				latestVersion := manifest.GoAgentVersion()
				if DetermineAgentVersionStatus(agentVersion, latestVersion) == "update_available" {
					d.SendUpdateRequest(conn)
				}
			}
		}
	}

	// Store connector discovery info in presence metadata (without overwriting connected_at/session_id).
	if len(data.Connectors) > 0 && d.PresenceStore != nil {
		meta := map[string]any{"connectors": data.Connectors}
		sessionID := strings.TrimSpace(conn.Meta("presence.session_id"))
		if sessionID != "" {
			_, _ = d.PresenceStore.UpdatePresenceMetadataForSession(conn.AssetID, sessionID, meta)
		} else {
			_ = d.PresenceStore.UpdatePresenceMetadata(conn.AssetID, meta)
		}
	}
}

func (d *Deps) ProcessAgentTelemetry(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.TelemetryData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid telemetry data from %s: %v", conn.AssetID, err)
		return
	}

	// Clamp percentage values to valid [0, 100] range.
	data.CPUPercent = shared.ClampPercent(data.CPUPercent)
	data.MemoryPercent = shared.ClampPercent(data.MemoryPercent)
	data.DiskPercent = shared.ClampPercent(data.DiskPercent)

	now := time.Now().UTC()
	samples := telemetry.BuildDirectSamples(conn.AssetID, now,
		data.CPUPercent, data.MemoryPercent, data.DiskPercent,
		data.NetRXBytesPerSec, data.NetTXBytesPerSec, data.TempCelsius,
	)

	if len(samples) > 0 {
		if err := d.TelemetryStore.AppendSamples(context.Background(), samples); err != nil {
			log.Printf("agentws: telemetry store append failed for %s: %v", conn.AssetID, err)
		}
	}
}

func (d *Deps) ProcessAgentCommandResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.CommandResultData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		assetID := ""
		if conn != nil {
			assetID = conn.AssetID
		}
		log.Printf("agentws: invalid command result from %s: %v", assetID, err)
		return
	}

	d.DeliverPendingAgentResult(conn, data)
}

func (d *Deps) ProcessAgentLogStream(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.LogStreamData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid log stream from %s: %v", conn.AssetID, err)
		return
	}

	if d.LogStore != nil {
		_ = d.LogStore.AppendEvent(logs.Event{
			AssetID: conn.AssetID,
			Source:  data.Source,
			Level:   data.Level,
			Message: data.Message,
		})
	}
}

func (d *Deps) ProcessAgentLogBatch(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.LogBatchData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid log batch from %s: %v", conn.AssetID, err)
		return
	}

	if d.LogStore != nil {
		events := make([]logs.Event, 0, len(data.Entries))
		for _, entry := range data.Entries {
			events = append(events, logs.Event{
				AssetID: conn.AssetID,
				Source:  entry.Source,
				Level:   entry.Level,
				Message: entry.Message,
			})
		}
		if batchAppender, ok := d.LogStore.(persistence.LogBatchAppendStore); ok {
			_ = batchAppender.AppendEvents(events)
			return
		}
		for _, event := range events {
			_ = d.LogStore.AppendEvent(event)
		}
	}
}

func (d *Deps) ProcessAgentUpdateProgress(_ *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.UpdateProgressData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}
	log.Printf("agentws: update progress job=%s stage=%s: %s", data.JobID, data.Stage, data.Message)
}

func (d *Deps) ProcessAgentUpdateResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.UpdateResultData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}

	output := strings.TrimSpace(data.Output)
	if strings.TrimSpace(data.Error) != "" {
		if output != "" {
			output += "\n"
		}
		output += strings.TrimSpace(data.Error)
	}
	d.DeliverPendingAgentResult(conn, agentmgr.CommandResultData{
		JobID:  data.JobID,
		Status: data.Status,
		Output: output,
	})
}

func (d *Deps) DeliverPendingAgentResult(conn *agentmgr.AgentConn, data agentmgr.CommandResultData) {
	jobID := strings.TrimSpace(data.JobID)
	if jobID == "" {
		return
	}
	raw, ok := d.PendingAgentCmds.Load(jobID)
	if !ok || raw == nil {
		return
	}

	send := func(ch chan agentmgr.CommandResultData, result agentmgr.CommandResultData) {
		select {
		case ch <- result:
		default:
		}
	}

	switch pending := raw.(type) {
	case PendingAgentCommand:
		if pending.ResultCh == nil {
			return
		}
		if !pending.AcceptsResult(conn, &data) {
			return
		}
		send(pending.ResultCh, data)
	case *PendingAgentCommand:
		if pending == nil || pending.ResultCh == nil {
			return
		}
		if !pending.AcceptsResult(conn, &data) {
			return
		}
		send(pending.ResultCh, data)
	}
}
