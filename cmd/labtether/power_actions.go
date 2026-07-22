package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
	"github.com/labtether/labtether/internal/powercontrol"
)

const powerActionRateLimit = 6

func (s *apiServer) ensurePowerCoordinator() *powercontrol.Coordinator {
	s.powerCoordinatorOnce.Do(func() {
		if s.powerCoordinator == nil {
			s.powerCoordinator = powercontrol.New(s.agentMgr, powercontrol.DefaultActionTimeout)
		}
	})
	return s.powerCoordinator
}

func (s *apiServer) processAgentPowerResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	if conn == nil {
		return
	}
	// The coordinator fails closed for malformed, stale, duplicate, or
	// cross-asset results. Avoid logging agent-provided payloads here.
	s.ensurePowerCoordinator().HandleResult(conn.AssetID, msg)
}

func (s *apiServer) handleV2AssetPowerAction(w http.ResponseWriter, r *http.Request, assetID string, rawAction string) {
	action := agentmgr.PowerAction(strings.TrimSpace(rawAction))
	if !action.Valid() {
		apiv2.WriteError(w, http.StatusBadRequest, "invalid_power_action", "unsupported power action")
		return
	}
	if !apiv2.AssetCheck(allowedAssetsFromContext(r.Context()), assetID) {
		apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "api key does not have access to this asset")
		return
	}
	if !maintenanceguard.EnforceAssetAction(w, assetID, s.ensureGroupFeaturesDeps().EvaluateAssetGuardrails) {
		s.auditPowerAction(r, assetID, action, agentmgr.PowerResultData{}, "denied", "maintenance_blocked")
		return
	}

	actorID := principalActorID(r.Context())
	bucket := fmt.Sprintf("assets.power:%s:%s:%s", actorID, assetID, action)
	if !s.enforceRateLimit(w, r, bucket, powerActionRateLimit, time.Minute) {
		s.auditPowerAction(r, assetID, action, agentmgr.PowerResultData{}, "rate_limited", "rate limit exceeded")
		return
	}

	result, err := s.ensurePowerCoordinator().Execute(r.Context(), assetID, action)
	if err != nil {
		kind := powercontrol.KindOf(err)
		status, code := powerHTTPError(kind)
		message := err.Error()
		if strings.TrimSpace(message) == "" {
			message = "power action failed"
		}
		reason := string(kind)
		if result.Code.Valid() {
			reason = string(result.Code)
		}
		s.auditPowerAction(r, assetID, action, result, "failed", reason)
		apiv2.WriteError(w, status, code, message)
		return
	}

	s.auditPowerAction(r, assetID, action, result, "accepted", "")
	statusText := "rebooting"
	if action == agentmgr.PowerActionShutdown {
		statusText = "shutting_down"
	}
	apiv2.WriteJSON(w, http.StatusAccepted, map[string]string{
		"status":     statusText,
		"asset_id":   assetID,
		"request_id": result.RequestID,
	})
}

func powerHTTPError(kind powercontrol.ErrorKind) (int, string) {
	switch kind {
	case powercontrol.ErrorInvalid:
		return http.StatusBadRequest, "invalid_power_action"
	case powercontrol.ErrorAgentOffline:
		return http.StatusConflict, "asset_offline"
	case powercontrol.ErrorUnsupported:
		return http.StatusUnprocessableEntity, "power_unsupported"
	case powercontrol.ErrorRejected:
		return http.StatusConflict, "power_rejected"
	case powercontrol.ErrorTimedOut:
		return http.StatusGatewayTimeout, "power_timed_out"
	case powercontrol.ErrorCanceled:
		return http.StatusRequestTimeout, "power_canceled"
	case powercontrol.ErrorBusy:
		return http.StatusTooManyRequests, "power_busy"
	case powercontrol.ErrorSendFailed:
		return http.StatusBadGateway, "power_delivery_failed"
	default:
		return http.StatusBadGateway, "power_execution_failed"
	}
}

func (s *apiServer) auditPowerAction(
	r *http.Request,
	assetID string,
	action agentmgr.PowerAction,
	result agentmgr.PowerResultData,
	decision string,
	reason string,
) {
	details := map[string]any{
		"action": string(action),
	}
	if result.Status.Valid() {
		details["agent_status"] = string(result.Status)
	}
	if result.Code.Valid() {
		details["agent_code"] = string(result.Code)
	}
	s.appendAuditEventBestEffort(audit.Event{
		Type:      "asset.power",
		ActorID:   principalActorID(r.Context()),
		Target:    assetID,
		CommandID: result.RequestID,
		Decision:  decision,
		Reason:    reason,
		Details:   details,
		Timestamp: time.Now().UTC(),
	}, "api warning: failed to append asset power audit event")
}
