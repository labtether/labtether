package proxmox

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// APIError is a Proxmox answer with a status of 300 or more. Its text stays
// "proxmox api returned <code>: <body>" so callers that read it keep working.
// Reason keeps Proxmox's own message: Proxmox puts it in the status line, and
// newer releases also in the body's "message".
type APIError struct {
	StatusCode int
	Reason     string
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("proxmox api returned %d: %s", e.StatusCode, e.Body)
}

func newAPIError(resp *http.Response, payload []byte) *APIError {
	reason := strings.TrimSpace(strings.TrimPrefix(resp.Status, strconv.Itoa(resp.StatusCode)))
	if reason == http.StatusText(resp.StatusCode) {
		reason = ""
	}
	var body struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(payload, &body) == nil && strings.TrimSpace(body.Message) != "" {
		reason = strings.TrimSpace(body.Message)
	}
	return &APIError{StatusCode: resp.StatusCode, Reason: reason, Body: strings.TrimSpace(string(payload))}
}
