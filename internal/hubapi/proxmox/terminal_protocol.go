package proxmox

import "strings"

// terminalControlMessage is the JSON control message format sent by the
// browser terminal component for resize and input events.
type terminalControlMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

// isControlMessage checks if a payload looks like a JSON control message.
func isControlMessage(payload []byte) bool {
	trimmed := strings.TrimSpace(string(payload))
	return len(trimmed) > 0 && trimmed[0] == '{'
}
