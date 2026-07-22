package resources

// heartbeat_handlers.go — HTTP handler and shared logic for asset heartbeat
// processing. Extracted from cmd/labtether/assets_heartbeat_handlers.go.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/platforms"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/telemetry"
)

const (
	heartbeatRateLimitBucket = "assets.heartbeat"
	heartbeatRateLimitCount  = 600
	heartbeatRateLimitWindow = time.Minute
)

// HandleRecordAssetHeartbeat handles POST /assets/{id}/heartbeat.
// It validates the payload, optionally validates the group_id, and delegates
// to ProcessHeartbeatRequest for the shared upsert + canonical model path.
func (d *Deps) HandleRecordAssetHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.EnforceRateLimit != nil && !d.EnforceRateLimit(w, r, heartbeatRateLimitBucket, heartbeatRateLimitCount, heartbeatRateLimitWindow) {
		return
	}

	var req assets.HeartbeatRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid heartbeat payload")
		return
	}

	req.AssetID = strings.TrimSpace(req.AssetID)
	req.Type = strings.TrimSpace(req.Type)
	req.Name = strings.TrimSpace(req.Name)
	req.Source = strings.TrimSpace(req.Source)
	req.GroupID = strings.TrimSpace(req.GroupID)
	req.Status = strings.TrimSpace(req.Status)
	req.Platform = platforms.Resolve(
		req.Platform,
		req.Metadata["platform"],
		req.Metadata["os"],
		req.Metadata["os_name"],
		req.Metadata["os_pretty_name"],
	)
	if req.Platform != "" {
		if req.Metadata == nil {
			req.Metadata = map[string]string{}
		}
		req.Metadata["platform"] = req.Platform
	}

	if req.AssetID == "" || req.Type == "" || req.Name == "" || req.Source == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset_id, type, name, and source are required")
		return
	}
	if !apiv2.RequireAssetAccess(w, r, req.AssetID) {
		return
	}
	if err := validateMaxLen("asset_id", req.AssetID, maxHeartbeatAssetIDLen); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateMaxLen("type", req.Type, maxHeartbeatTypeLen); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateMaxLen("name", req.Name, maxHeartbeatNameLen); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateMaxLen("source", req.Source, maxHeartbeatSourceLen); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Group placement is operator-owned after enrollment. Identity-bound agent
	// heartbeats preserve the asset's stored group transactionally, so ignore a
	// stale client-side value before validating ordinary mutable heartbeats.
	identityBoundAgentHeartbeat := shared.AgentTokenIDFromContext(r.Context()) != "" ||
		strings.EqualFold(req.Source, "agent") && shared.ExistingAgentHeartbeatOnlyFromContext(r.Context())
	if identityBoundAgentHeartbeat {
		req.GroupID = ""
	} else if req.GroupID != "" && d.GroupStore != nil {
		_, ok, err := d.GroupStore.GetGroup(req.GroupID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to validate group")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusBadRequest, "group_id does not reference an existing group")
			return
		}
	}

	var assetEntry *assets.Asset
	var err error
	if agentTokenID := shared.AgentTokenIDFromContext(r.Context()); agentTokenID != "" {
		if d.EnrollmentTransactions == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "agent heartbeat transaction unavailable")
			return
		}
		commitCtx, cancelCommit := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancelCommit()
		committed, commitErr := d.EnrollmentTransactions.CommitAuthenticatedAgentHeartbeat(commitCtx, agentTokenID, req)
		if commitErr != nil {
			if errors.Is(commitErr, persistence.ErrAgentCredentialInactive) || errors.Is(commitErr, persistence.ErrNotFound) {
				servicehttp.WriteError(w, http.StatusUnauthorized, "agent credential is no longer active")
			} else {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to record heartbeat")
			}
			return
		}
		assetEntry = &committed
		d.processCommittedHeartbeatSideEffects(committed, req)
	} else if strings.EqualFold(req.Source, "agent") && shared.SharedAgentHeartbeatDisabledFromContext(r.Context()) {
		servicehttp.WriteError(w, http.StatusUnauthorized, "per-agent credential required for agent heartbeat")
		return
	} else if strings.EqualFold(req.Source, "agent") && shared.ExistingAgentHeartbeatOnlyFromContext(r.Context()) {
		// Owner/admin/API-key authority may refresh a legacy agent asset, but it
		// must not recreate an identity that was never enrolled or that an
		// operator decommissioned. This path also strips identity TOFU metadata in
		// the persistence transaction.
		if d.EnrollmentTransactions == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "agent heartbeat transaction unavailable")
			return
		}
		commitCtx, cancelCommit := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancelCommit()
		committed, commitErr := d.EnrollmentTransactions.CommitExistingOwnerAgentHeartbeat(commitCtx, req)
		if commitErr != nil {
			if errors.Is(commitErr, persistence.ErrNotFound) {
				servicehttp.WriteError(w, http.StatusConflict, "agent asset must already be enrolled")
			} else {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to record heartbeat")
			}
			return
		}
		assetEntry = &committed
		d.processCommittedHeartbeatSideEffects(committed, req)
	} else {
		assetEntry, err = d.ProcessHeartbeatRequest(req)
	}
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to record heartbeat")
		return
	}

	servicehttp.WriteJSON(w, http.StatusAccepted, map[string]any{
		"status": "recorded",
		"asset":  assetEntry,
	})
}

// ProcessHeartbeatRequest is the shared heartbeat processing logic used by
// both the HTTP POST handler and the WebSocket agent handler. It upserts the
// asset heartbeat, persists canonical state, and appends telemetry samples.
func (d *Deps) ProcessHeartbeatRequest(req assets.HeartbeatRequest) (*assets.Asset, error) {
	assetEntry, err := d.AssetStore.UpsertAssetHeartbeat(req)
	if err != nil {
		return nil, fmt.Errorf("upsert asset heartbeat: %w", err)
	}

	d.processCommittedHeartbeatSideEffects(assetEntry, req)
	return &assetEntry, nil
}

func (d *Deps) processCommittedHeartbeatSideEffects(assetEntry assets.Asset, req assets.HeartbeatRequest) {
	if d.PersistCanonicalHeartbeatFn != nil {
		d.PersistCanonicalHeartbeatFn(assetEntry, req)
	}
	samples := telemetry.SamplesFromHeartbeatMetadata(assetEntry.ID, assetEntry.LastSeenAt, req.Metadata)
	if len(samples) > 0 && d.TelemetryStore != nil {
		if err := d.TelemetryStore.AppendSamples(context.Background(), samples); err != nil {
			log.Printf("api warning: failed to append telemetry samples for %s: %v", assetEntry.ID, err)
		}
	}

}

// Validation length constants for heartbeat fields. These mirror the constants
// in cmd/labtether (maxTargetLength, maxModeLength, etc.) so that the handler
// enforces the same limits after extraction.
const (
	maxHeartbeatAssetIDLen = 255
	maxHeartbeatTypeLen    = 32
	maxHeartbeatNameLen    = 120
	maxHeartbeatSourceLen  = 64
)
