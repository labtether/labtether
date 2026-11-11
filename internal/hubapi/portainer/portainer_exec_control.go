package portainer

import (
	"encoding/json"
	"strings"
)

// portainerExecControl reads a control message the console's terminal sends
// as a text frame ({"type":"resize","cols":…,"rows":…}). Portainer's exec
// WebSocket carries only the shell's input, so a control message must never
// be passed on: it would reach the shell as typed text.
type portainerExecControl struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
	Data string `json:"data"`
}

// parsePortainerExecControl reports whether payload is a terminal control
// message, and returns it.
func parsePortainerExecControl(payload []byte) (portainerExecControl, bool) {
	trimmed := strings.TrimSpace(string(payload))
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return portainerExecControl{}, false
	}
	var control portainerExecControl
	if err := json.Unmarshal([]byte(trimmed), &control); err != nil {
		return portainerExecControl{}, false
	}
	switch strings.ToLower(strings.TrimSpace(control.Type)) {
	case "resize", "ping", "input":
		control.Type = strings.ToLower(strings.TrimSpace(control.Type))
		return control, true
	default:
		return portainerExecControl{}, false
	}
}
