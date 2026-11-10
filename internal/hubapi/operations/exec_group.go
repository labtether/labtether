package operations

import (
	"net/http"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/persistence"
)

// execGroupTargets lists the devices of the group a multi-device exec names.
// A group ID that names no group, or a group without devices, is answered in
// those words; both used to get "targets or group required", which tells the
// user nothing about the group they gave. It reports false once it has
// written an answer.
func (d *ExecDeps) execGroupTargets(w http.ResponseWriter, groupID string) ([]string, bool) {
	if d.GroupStore != nil {
		_, found, err := d.GroupStore.GetGroup(groupID)
		if err != nil {
			apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to load group: "+err.Error())
			return nil, false
		}
		if !found {
			apiv2.WriteError(w, http.StatusNotFound, "not_found", "group not found")
			return nil, false
		}
	}
	groupAssetStore, ok := d.AssetStore.(persistence.GroupAssetStore)
	if !ok {
		return nil, true
	}
	members, err := groupAssetStore.ListAssetsByGroup(groupID)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to list group devices: "+err.Error())
		return nil, false
	}
	targets := make([]string, 0, min(len(members), maxExecMultiUniqueTargets+1))
	seen := make(map[string]struct{}, min(len(members), maxExecMultiUniqueTargets+1))
	for _, member := range members {
		if err := appendExecMultiTarget(&targets, seen, member.ID); err != nil {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", err.Error())
			return nil, false
		}
	}
	if len(targets) == 0 {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "group has no devices")
		return nil, false
	}
	return targets, true
}
