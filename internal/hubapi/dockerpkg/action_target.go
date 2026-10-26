package dockerpkg

import (
	"context"
	"errors"
	"net/http"

	"github.com/labtether/labtether/internal/connectors/docker"
	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) authorizeDockerHostActionTarget(w http.ResponseWriter, r *http.Request, host *docker.DockerHost, actionID, targetID string) (context.Context, bool) {
	target, err := d.DockerCoordinator.ResolveActionTarget(actionID, targetID)
	if err != nil {
		message := "invalid docker action target"
		if errors.Is(err, docker.ErrComposeUpgradeRequired) {
			message = docker.ErrComposeUpgradeRequired.Error()
		}
		servicehttp.WriteError(w, http.StatusBadRequest, message)
		return nil, false
	}
	if target.AgentID != host.AgentID {
		servicehttp.WriteError(w, http.StatusForbidden, "action target does not belong to the selected docker host")
		return nil, false
	}
	hostAssetID := "docker-host-" + NormalizeDockerHostLookupID(host.AgentID)
	if target.AssetID != hostAssetID {
		if !requireDockerAssetAccess(w, r, target.AssetID) || !d.enforceAssetActionGuard(w, target.AssetID) {
			return nil, false
		}
	}
	if !d.enforceAssetActionGuard(w, hostAssetID) {
		return nil, false
	}
	return docker.WithActionTarget(r.Context(), target), true
}

func (d *Deps) authorizeDockerStackActionTarget(w http.ResponseWriter, r *http.Request, actionID, stackAssetID string) (context.Context, bool) {
	target, err := d.DockerCoordinator.ResolveActionTarget(actionID, stackAssetID)
	if err != nil || target.AssetID != stackAssetID {
		message := "invalid docker stack action target"
		if errors.Is(err, docker.ErrComposeUpgradeRequired) {
			message = docker.ErrComposeUpgradeRequired.Error()
		}
		servicehttp.WriteError(w, http.StatusBadRequest, message)
		return nil, false
	}
	if !d.enforceAssetActionGuard(w, target.AgentID) || !d.enforceAssetActionGuard(w, "docker-host-"+NormalizeDockerHostLookupID(target.AgentID)) {
		return nil, false
	}
	return docker.WithActionTarget(r.Context(), target), true
}
