package truenas

import (
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/connectorsdk"
)

// ResourceAssetID is shared by discovery and action maintenance lookup. It
// identifies an existing resource only; callers must resolve collector ownership.
func ResourceAssetID(kind, name string) string {
	if strings.TrimSpace(name) == "" {
		return ""
	}
	switch kind {
	case "storage-pool", "dataset", "disk", "service", "app", "share-smb", "share-nfs":
		return "truenas-" + kind + "-" + normalizeID(name)
	default:
		return ""
	}
}

// ActionResourceAssetIDs follows the same parameters/fallbacks as dispatch.
// Dataset parents and the pool are storage owners of child creation/rollback.
// Share identities come from an exact provider lookup in the dedicated handler.
// Replication uses separate paged local-owner checks, never remote name guesses.
func ActionResourceAssetIDs(action string, req connectorsdk.ActionRequest) ([]string, error) {
	var ids []string
	var lookupErr error
	add := func(kind, name string) {
		if id := ResourceAssetID(kind, name); id != "" {
			ids = append(ids, id)
		}
	}
	dataset := func(name string) {
		name = strings.TrimSpace(name)
		// Bound stored-asset lookups even for a malformed deeply nested request.
		if len(name) > 4096 || strings.Count(name, "/") >= 64 {
			lookupErr = fmt.Errorf("dataset ownership path exceeds lookup limit")
			return
		}
		if name == "" {
			return
		}
		pool, _, _ := strings.Cut(name, "/")
		add("storage-pool", pool)
		for name != "" {
			add("dataset", name)
			i := strings.LastIndexByte(name, '/')
			if i < 0 {
				break
			}
			name = name[:i]
		}
	}
	switch action {
	case "pool.scrub":
		add("storage-pool", paramOrTarget(req, "pool_name"))
	case "dataset.create", "dataset.update", "dataset.delete", "snapshot.create":
		dataset(req.Params["dataset"])
	case "snapshot.delete", "snapshot.rollback", "snapshot.clone":
		snapshot := paramOrTarget(req, "snapshot_id")
		if name, _, found := strings.Cut(snapshot, "@"); found {
			dataset(name)
		}
		if action == "snapshot.clone" {
			dataset(req.Params["dataset_dst"])
		}
	case "service.start", "service.stop", "service.restart", "service.update":
		add("service", paramOrTarget(req, "service"))
	case "app.start", "app.stop", "app.restart":
		add("app", paramOrTarget(req, "app_name"))
	case "share.create", "share.update", "share.delete":
		kind := req.Params["share_type"]
		if kind == "smb" || kind == "nfs" {
			add("share-"+kind, req.Params["share_identity"])
			add("share-"+kind, req.Params["share_destination"])
		}
	case "smart.test":
		disk := strings.TrimSpace(req.Params["disk"])
		if disk == "" {
			disk = strings.TrimPrefix(assetid.NativeCollectorAssetID(req.TargetID), "truenas-disk-")
		}
		add("disk", disk)
	}
	return ids, lookupErr
}
