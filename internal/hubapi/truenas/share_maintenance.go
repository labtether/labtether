package truenas

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

type trueNASShareIdentity struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

func trueNASShareName(kind string, share trueNASShareIdentity) string {
	if kind == "smb" {
		if name := strings.TrimSpace(share.Name); name != "" {
			return name
		}
		if share.ID > 0 {
			return strconv.Itoa(share.ID)
		}
	} else if kind == "nfs" {
		if path := strings.TrimSpace(share.Path); path != "" {
			return path
		}
		if share.ID > 0 {
			return "nfs-" + strconv.Itoa(share.ID)
		}
	}
	return ""
}

// Resolve the API's numeric identity on the exact appliance before checking
// stored discovery names. No request parameter can replace this provider read.
func (d *Deps) enforceShareMutation(ctx context.Context, w http.ResponseWriter, asset assets.Asset, runtime *TruenasRuntime, kind, action string, id int, params map[string]any) bool {
	if !d.enforceMutation(w, asset, runtime, action, nil) {
		return false
	}
	share := trueNASShareIdentity{ID: id}
	original := ""
	if id > 0 {
		var rows []trueNASShareIdentity
		query := []any{[]any{[]any{"id", "=", id}}, map[string]any{"limit": 2, "select": []string{"id", "name", "path"}}}
		if kind == "nfs" {
			query[1] = map[string]any{"limit": 2, "select": []string{"id", "path"}}
		}
		if err := runtime.Client.Call(ctx, "sharing."+kind+".query", query, &rows); err != nil {
			writeTrueNASError(w, http.StatusBadGateway, "cannot verify share identity", err)
			return false
		}
		if len(rows) != 1 || rows[0].ID != id {
			servicehttp.WriteError(w, http.StatusConflict, "share identity is missing or ambiguous")
			return false
		}
		share = rows[0]
		original = trueNASShareName(kind, share)
		// Stable metadata also finds a previously discovered share whose name
		// changed on the NAS since discovery. Current name lookup remains for
		// legacy assets that do not yet carry share_id.
		lookup, ok := d.AssetStore.(persistence.TrueNASShareAssetStore)
		if !ok {
			servicehttp.WriteError(w, http.StatusConflict, "share inventory lookup unavailable")
			return false
		}
		owners, err := lookup.FindTrueNASShareAssets(ctx, runtime.CollectorID, kind, id)
		if err != nil || len(owners) > 2 {
			servicehttp.WriteError(w, http.StatusConflict, "share inventory identity is ambiguous")
			return false
		}
		for _, owner := range owners {
			if !d.enforceMutation(w, owner, runtime, action, nil) {
				return false
			}
		}
	}
	for key, target := range map[string]*string{"name": &share.Name, "path": &share.Path} {
		if value, found := params[key]; found {
			text, ok := value.(string)
			if !ok || strings.TrimSpace(text) == "" {
				servicehttp.WriteError(w, http.StatusBadRequest, fmt.Sprintf("share %s must be a nonempty string", key))
				return false
			}
			*target = text
		}
	}
	destination := trueNASShareName(kind, share)
	if destination == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "share name or path is required")
		return false
	}
	if err := ctx.Err(); err != nil {
		writeTrueNASError(w, http.StatusRequestTimeout, "share check canceled before mutation", err)
		return false
	}
	return d.enforceMutation(w, asset, runtime, action, map[string]string{
		"share_type": kind, "share_identity": original, "share_destination": destination,
	})
}
