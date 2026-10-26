package actionspkg

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
	"github.com/labtether/labtether/internal/servicehttp"
)

// Bind after authorization, before queueing: a raw VM ID must not move to a
// different provider if the collector configuration changes while this job waits.
func (d *Deps) bindConnectorActionCollector(ctx context.Context, w http.ResponseWriter, req *actions.ExecuteRequest) bool {
	if configured, ok := d.ConfiguredConnectorActions[req.ConnectorID]; ok {
		if configured.Bind == nil || configured.Execute == nil {
			servicehttp.WriteError(w, http.StatusConflict, "configured connector action unavailable")
			return false
		}
		bound, err := configured.Bind(ctx, req.ActionID, connectorsdk.ActionRequest{TargetID: req.Target, Params: req.Params, DryRun: req.DryRun})
		if err != nil {
			if errors.Is(err, connectorsdk.ErrComposeUpgradeRequired) {
				servicehttp.WriteError(w, http.StatusConflict, connectorsdk.ErrComposeUpgradeRequired.Error())
			} else if !maintenanceguard.WriteBlockedAction(w, err) {
				servicehttp.WriteError(w, http.StatusConflict, "cannot bind connector action target")
			}
			return false
		}
		if bound.TargetID != req.Target || bound.DryRun != req.DryRun {
			servicehttp.WriteError(w, http.StatusConflict, "connector binding changed the logical request")
			return false
		}
		req.Params = bound.Params
		return true
	}
	var id string
	var err error
	switch {
	case req.ConnectorID == "proxmox" && d.ResolveProxmoxActionCollector != nil:
		id, err = d.ResolveProxmoxActionCollector(req.ActionID, req.Target, req.Params)
	default:
		return true
	}
	if err != nil || strings.TrimSpace(id) == "" {
		servicehttp.WriteError(w, http.StatusConflict, "cannot resolve action collector")
		return false
	}
	req.Params = maps.Clone(req.Params)
	if req.Params == nil {
		req.Params = make(map[string]string)
	}
	req.Params["collector_id"] = id
	return true
}
