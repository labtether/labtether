package terminal

import (
	"encoding/json"
	"testing"
)

// Risk: clearing a deleted device from saved workspace tabs must not delete a
// split tab that still shows another device, or move that device's pane.
func TestWorkspacePanesWithoutDeviceKeepsOtherDevices(t *testing.T) {
	split := json.RawMessage(`[{"targetNodeId":"nas01","tmuxSession":"main"},{"targetNodeId":"pve01"}]`)
	next, keep, err := workspacePanesWithoutDevice(split, "nas01")
	if err != nil || !keep {
		t.Fatalf("split tab: keep=%t err=%v, want the tab kept", keep, err)
	}
	var panes []map[string]string
	if err := json.Unmarshal(next, &panes); err != nil {
		t.Fatalf("decode panes: %v", err)
	}
	if len(panes) != 2 || panes[0]["targetNodeId"] != "" || panes[0]["tmuxSession"] != "" || panes[1]["targetNodeId"] != "pve01" {
		t.Fatalf("split tab panes = %s, want an empty first pane and pve01 second", next)
	}

	only := json.RawMessage(`[{"targetNodeId":"nas01"},{"targetNodeId":""}]`)
	if _, keep, err := workspacePanesWithoutDevice(only, "nas01"); err != nil || keep {
		t.Fatalf("tab showing only the deleted device: keep=%t err=%v, want it removed", keep, err)
	}
}
