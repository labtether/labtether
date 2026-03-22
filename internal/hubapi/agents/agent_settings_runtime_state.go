package agents

import (
	"encoding/json"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentcore"
	"github.com/labtether/labtether/internal/agentmgr"
)

type AgentSettingsRuntimeState struct {
	Status               string
	Revision             string
	LastError            string
	UpdatedAt            time.Time
	AppliedAt            time.Time
	RestartRequired      bool
	AllowRemoteOverrides bool
	Fingerprint          string
	Values               map[string]string
}

func (d *Deps) PushAgentSettingsApply(assetID string, values map[string]string) {
	if d.AgentMgr == nil {
		return
	}
	conn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		return
	}

	expectedFingerprint := ""
	if d.AssetStore != nil {
		if asset, exists, err := d.AssetStore.GetAsset(assetID); err == nil && exists {
			expectedFingerprint = strings.TrimSpace(asset.Metadata["agent_device_fingerprint"])
		}
	}

	requestID := shared.GenerateRequestID()
	revision := time.Now().UTC().Format(time.RFC3339Nano)
	payload := agentmgr.AgentSettingsApplyData{
		RequestID:           requestID,
		Revision:            revision,
		Values:              CloneAgentSettingValues(values),
		ExpectedFingerprint: expectedFingerprint,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	if err := conn.Send(agentmgr.Message{
		Type: agentmgr.MsgAgentSettingsApply,
		Data: data,
	}); err != nil {
		d.SetAgentSettingsRuntimeState(assetID, AgentSettingsRuntimeState{
			Status:      "send_failed",
			Revision:    revision,
			LastError:   err.Error(),
			UpdatedAt:   time.Now().UTC(),
			Fingerprint: expectedFingerprint,
		})
		return
	}

	d.SetAgentSettingsRuntimeState(assetID, AgentSettingsRuntimeState{
		Status:      "pending",
		Revision:    revision,
		UpdatedAt:   time.Now().UTC(),
		Fingerprint: expectedFingerprint,
		Values:      CloneAgentSettingValues(values),
	})
}

func (d *Deps) SendAgentSettingsApply(conn *agentmgr.AgentConn) {
	if conn == nil {
		return
	}
	effective, err := d.CollectEffectiveAgentSettingValues(conn.AssetID)
	if err != nil {
		return
	}
	for key := range effective {
		if def, ok := agentcore.AgentSettingDefinitionByKey(key); ok && def.LocalOnly {
			delete(effective, key)
		}
	}
	d.PushAgentSettingsApply(conn.AssetID, effective)
}

func (d *Deps) ProcessAgentSettingsApplied(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.AgentSettingsAppliedData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}
	status := "applied"
	if !data.Applied {
		status = "failed"
	}
	state := AgentSettingsRuntimeState{
		Status:          status,
		Revision:        strings.TrimSpace(data.Revision),
		LastError:       strings.TrimSpace(data.Error),
		UpdatedAt:       time.Now().UTC(),
		RestartRequired: data.RestartRequired,
		Fingerprint:     strings.TrimSpace(data.Fingerprint),
		Values:          CloneAgentSettingValues(data.AppliedValues),
	}
	if ts, err := time.Parse(time.RFC3339, strings.TrimSpace(data.AppliedAt)); err == nil {
		state.AppliedAt = ts.UTC()
	}
	d.SetAgentSettingsRuntimeState(conn.AssetID, state)
}

func (d *Deps) ProcessAgentSettingsState(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.AgentSettingsStateData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}
	state := AgentSettingsRuntimeState{
		Status:               "reported",
		Revision:             strings.TrimSpace(data.Revision),
		UpdatedAt:            time.Now().UTC(),
		AllowRemoteOverrides: data.AllowRemoteOverrides,
		Fingerprint:          strings.TrimSpace(data.Fingerprint),
		Values:               CloneAgentSettingValues(data.Values),
	}
	if ts, err := time.Parse(time.RFC3339, strings.TrimSpace(data.ReportedAt)); err == nil {
		state.UpdatedAt = ts.UTC()
	}
	if previous, ok := d.GetAgentSettingsRuntimeState(conn.AssetID); ok {
		state = MergeAgentSettingsReportState(previous, state)
	}
	d.SetAgentSettingsRuntimeState(conn.AssetID, state)
}

func MergeAgentSettingsReportState(previous, reported AgentSettingsRuntimeState) AgentSettingsRuntimeState {
	if SameAgentSettingsRevision(previous.Revision, reported.Revision) && PreservesAgentSettingsApplyStatus(previous.Status) {
		reported.Status = previous.Status
		reported.LastError = previous.LastError
		reported.AppliedAt = previous.AppliedAt
		reported.RestartRequired = previous.RestartRequired
	}
	return reported
}

func SameAgentSettingsRevision(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	return left != "" && left == right
}

func PreservesAgentSettingsApplyStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "applied", "failed":
		return true
	default:
		return false
	}
}

func (d *Deps) GetAgentSettingsRuntimeState(assetID string) (AgentSettingsRuntimeState, bool) {
	if value, ok := d.AgentSettingsState.Load(assetID); ok {
		if state, ok := value.(AgentSettingsRuntimeState); ok {
			return state, true
		}
	}
	return AgentSettingsRuntimeState{}, false
}

func (d *Deps) SetAgentSettingsRuntimeState(assetID string, state AgentSettingsRuntimeState) {
	d.AgentSettingsState.Store(assetID, state)
}
