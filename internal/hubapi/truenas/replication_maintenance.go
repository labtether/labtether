package truenas

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

type trueNASReplicationIdentity struct {
	ID        int      `json:"id"`
	Direction string   `json:"direction"`
	Transport string   `json:"transport"`
	Sources   []string `json:"source_datasets"`
	Target    string   `json:"target_dataset"`
	Recursive *bool    `json:"recursive"`
	Exclude   []string `json:"exclude"`
}

func validTrueNASDatasetName(name string) bool {
	if name == "" || len(name) > 4096 || strings.Count(name, "/") >= 64 || strings.TrimSpace(name) != name || strings.ContainsAny(name, "@#\x00") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func trueNASDatasetExcluded(name string, excluded []string) bool {
	for _, path := range excluded {
		if name == path || strings.HasPrefix(name, path+"/") {
			return true
		}
	}
	return false
}

func (d *Deps) enforceReplicationMutation(ctx context.Context, w http.ResponseWriter, asset assets.Asset, runtime *TruenasRuntime, id int) bool {
	if !d.enforceMutation(w, asset, runtime, "replication.run", nil) {
		return false
	}
	var rows []trueNASReplicationIdentity
	query := []any{[]any{[]any{"id", "=", id}}, map[string]any{"limit": 2, "select": []string{
		"id", "direction", "transport", "source_datasets", "target_dataset", "recursive", "exclude",
	}}}
	if err := runtime.Client.Call(ctx, "replication.query", query, &rows); err != nil {
		writeTrueNASError(w, http.StatusBadGateway, "cannot verify replication identity", err)
		return false
	}
	if len(rows) != 1 || rows[0].ID != id || rows[0].Recursive == nil {
		servicehttp.WriteError(w, http.StatusConflict, "replication identity is missing or ambiguous")
		return false
	}
	task := rows[0]
	if (task.Direction != "PUSH" && task.Direction != "PULL") ||
		(task.Transport != "LOCAL" && task.Transport != "SSH" && task.Transport != "SSH+NETCAT") || len(task.Sources) == 0 {
		servicehttp.WriteError(w, http.StatusConflict, "replication locality cannot be verified")
		return false
	}
	for _, name := range append(append([]string{task.Target}, task.Sources...), task.Exclude...) {
		if !validTrueNASDatasetName(name) {
			servicehttp.WriteError(w, http.StatusConflict, "replication dataset identity is invalid")
			return false
		}
	}
	// The provider's FSAttachmentDelegate uses precisely these locality rules.
	if task.Transport == "LOCAL" || task.Direction == "PUSH" {
		for _, root := range task.Sources {
			if !d.guardReplicationDataset(ctx, w, asset, runtime, root, *task.Recursive, task.Exclude) {
				return false
			}
		}
	}
	if task.Transport == "LOCAL" || task.Direction == "PULL" {
		// zettarepl maps relative paths from the source common ancestor into
		// target_dataset. Exclusions must be translated by that same mapping.
		base := trueNASSourceBase(task.Sources)
		var excluded []string
		for _, source := range task.Exclude {
			if base == "" || source == base || strings.HasPrefix(source, base+"/") {
				excluded = append(excluded, trueNASReplicationTarget(task.Target, base, source))
			}
		}
		for _, source := range task.Sources {
			target := trueNASReplicationTarget(task.Target, base, source)
			if !d.guardReplicationDataset(ctx, w, asset, runtime, target, *task.Recursive, excluded) {
				return false
			}
		}
	}
	if err := ctx.Err(); err != nil {
		writeTrueNASError(w, http.StatusRequestTimeout, "replication check canceled before mutation", err)
		return false
	}
	return d.enforceMutation(w, asset, runtime, "replication.run", nil)
}

func trueNASSourceBase(sources []string) string {
	base := strings.Split(sources[0], "/")
	for _, source := range sources[1:] {
		parts := strings.Split(source, "/")
		i := 0
		for i < len(base) && i < len(parts) && base[i] == parts[i] {
			i++
		}
		base = base[:i]
	}
	return strings.Join(base, "/")
}

func trueNASReplicationTarget(target, base, source string) string {
	if source == base {
		return target
	}
	if base == "" {
		return target + "/" + source
	}
	return target + "/" + strings.TrimPrefix(source, base+"/")
}

func (d *Deps) guardReplicationDataset(ctx context.Context, w http.ResponseWriter, asset assets.Asset, runtime *TruenasRuntime, root string, recursive bool, excluded []string) bool {
	if err := ctx.Err(); err != nil {
		writeTrueNASError(w, http.StatusRequestTimeout, "replication owner check timed out", err)
		return false
	}
	if trueNASDatasetExcluded(root, excluded) {
		return true
	}
	if !d.enforceMutation(w, asset, runtime, "dataset.update", map[string]string{"dataset": root}) {
		return false
	}
	if !recursive {
		return true
	}
	lookup, ok := d.AssetStore.(persistence.TrueNASDatasetAssetStore)
	if !ok {
		servicehttp.WriteError(w, http.StatusConflict, "dataset inventory lookup unavailable")
		return false
	}
	after := ""
	for {
		rows, err := lookup.ListTrueNASDatasetAssets(ctx, runtime.CollectorID, root, after, 128)
		if err != nil {
			writeTrueNASError(w, http.StatusConflict, "cannot verify replication dataset owners", err)
			return false
		}
		for _, row := range rows {
			if err := ctx.Err(); err != nil {
				writeTrueNASError(w, http.StatusRequestTimeout, "replication owner check timed out", err)
				return false
			}
			name, err := trueNASStoredDatasetName(row, runtime.CollectorID)
			if err != nil || row.ID <= after || (name != root && !strings.HasPrefix(name, root+"/")) {
				servicehttp.WriteError(w, http.StatusConflict, "replication dataset owner is ambiguous")
				return false
			}
			after = row.ID
			if trueNASDatasetExcluded(name, excluded) {
				continue
			}
			if !d.enforceMutation(w, row, runtime, "dataset.update", map[string]string{"dataset": name}) {
				return false
			}
		}
		if err := ctx.Err(); err != nil {
			writeTrueNASError(w, http.StatusRequestTimeout, "replication owner check timed out", err)
			return false
		}
		if len(rows) < 128 {
			return true
		}
	}
}

func trueNASStoredDatasetName(row assets.Asset, collectorID string) (string, error) {
	owner := strings.TrimSpace(row.Metadata["collector_id"])
	scope, scoped := assetid.CollectorScopeFromAssetID(row.ID)
	if row.Source != "truenas" || row.Type != "dataset" ||
		(scoped && (collectorID == "" || scope != assetid.CollectorScope(collectorID))) ||
		(owner != "" && owner != collectorID) || (!scoped && owner != collectorID) {
		return "", fmt.Errorf("dataset collector is ambiguous")
	}
	name := row.Metadata["dataset_name"]
	if name == "" {
		if row.Metadata[assets.MetadataKeyNameOverride] != "" {
			return "", fmt.Errorf("renamed legacy dataset has no provider identity")
		}
		name = row.Name
	}
	if !validTrueNASDatasetName(name) {
		return "", fmt.Errorf("invalid stored dataset identity")
	}
	return name, nil
}
