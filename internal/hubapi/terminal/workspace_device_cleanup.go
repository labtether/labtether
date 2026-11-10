package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// workspacePaneTargetKey is the pane field the console stores a device id in.
const workspacePaneTargetKey = "targetNodeId"

type workspaceTabPanes struct {
	id    string
	panes json.RawMessage
}

// clearDeviceFromWorkspaceTabs removes a deleted device from every user's
// saved workspace tabs. A tab that showed only that device is deleted; in a
// split tab the device's panes are emptied and the other panes stay.
func (d *Deps) clearDeviceFromWorkspaceTabs(ctx context.Context, assetID string) error {
	if d.DBPool == nil {
		return nil
	}
	match, err := json.Marshal([]map[string]string{{workspacePaneTargetKey: assetID}})
	if err != nil {
		return err
	}
	rows, err := d.DBPool.Query(ctx,
		`SELECT id, panes FROM terminal_workspace_tabs WHERE panes @> $1::jsonb`, string(match))
	if err != nil {
		if terminalWorkspaceSchemaMissing(err) {
			return nil
		}
		return err
	}
	var tabs []workspaceTabPanes
	for rows.Next() {
		var tab workspaceTabPanes
		if err := rows.Scan(&tab.id, &tab.panes); err != nil {
			rows.Close()
			return err
		}
		tabs = append(tabs, tab)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	var failures []error
	for _, tab := range tabs {
		panes, keep, err := workspacePanesWithoutDevice(tab.panes, assetID)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if keep {
			_, err = d.DBPool.Exec(ctx,
				`UPDATE terminal_workspace_tabs SET panes = $2, updated_at = $3 WHERE id = $1`,
				tab.id, panes, time.Now().UTC())
		} else {
			_, err = d.DBPool.Exec(ctx, `DELETE FROM terminal_workspace_tabs WHERE id = $1`, tab.id)
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// workspacePanesWithoutDevice empties the panes that point at assetID and
// keeps their positions, so a split layout does not shift. keep is false when
// no pane points at another device, which means the tab itself can go.
func workspacePanesWithoutDevice(raw json.RawMessage, assetID string) (json.RawMessage, bool, error) {
	var panes []map[string]any
	if err := json.Unmarshal(raw, &panes); err != nil {
		return nil, false, err
	}
	keep := false
	for index, pane := range panes {
		target, _ := pane[workspacePaneTargetKey].(string)
		switch target {
		case assetID:
			panes[index] = map[string]any{workspacePaneTargetKey: ""}
		case "":
		default:
			keep = true
		}
	}
	if !keep {
		return nil, false, nil
	}
	next, err := json.Marshal(panes)
	return next, keep, err
}
