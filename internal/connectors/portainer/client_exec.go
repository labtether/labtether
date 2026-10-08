package portainer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// CreateExec creates a TTY exec instance in a container and returns the exec ID.
// cmd must be non-empty; if nil or empty, ["/bin/sh"] is used.
func (c *Client) CreateExec(ctx context.Context, endpointID int, containerID string, cmd []string) (string, error) {
	if len(cmd) == 0 {
		cmd = []string{"/bin/sh"}
	}
	path := fmt.Sprintf("/api/endpoints/%s/docker/containers/%s/exec",
		url.PathEscape(fmt.Sprintf("%d", endpointID)),
		url.PathEscape(containerID),
	)
	body := map[string]any{
		"AttachStdin":  true,
		"AttachStdout": true,
		"AttachStderr": true,
		"Tty":          true,
		"Cmd":          cmd,
	}
	payload, err := c.post(ctx, path, body)
	if err != nil {
		return "", fmt.Errorf("create exec instance: %w", err)
	}
	execID, err := decodePortainerExecID(payload)
	if err != nil {
		return "", fmt.Errorf("decode exec response: %w", err)
	}
	return execID, nil
}

func decodePortainerExecID(payload []byte) (string, error) {
	var result map[string]json.RawMessage
	if err := json.Unmarshal(payload, &result); err != nil {
		return "", err
	}
	var selected string
	for _, key := range []string{"Id", "id", "execId"} {
		raw, ok := result[key]
		if !ok {
			continue
		}
		var candidate string
		if err := json.Unmarshal(raw, &candidate); err != nil {
			return "", fmt.Errorf("%s must be a string", key)
		}
		if candidate == "" {
			continue
		}
		if selected != "" && candidate != selected {
			return "", fmt.Errorf("conflicting exec IDs in response")
		}
		selected = candidate
	}
	if err := validatePortainerExecID(selected); err != nil {
		return "", err
	}
	return selected, nil
}

func validatePortainerExecID(execID string) error {
	if len(execID) != portainerExecIDLength {
		return fmt.Errorf("portainer exec ID must be %d hexadecimal characters", portainerExecIDLength)
	}
	for _, char := range execID {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') && (char < 'A' || char > 'F') {
			return fmt.Errorf("portainer exec ID must be hexadecimal")
		}
	}
	return nil
}

// ExecWebSocketURL returns the WebSocket URL and auth token for connecting to an
// exec session via Portainer's /api/websocket/exec endpoint. The token must be
// sent in an Authorization header; keeping it out of the URL avoids exposing it
// through access logs and is required by current Portainer releases.
// wsURL uses the ws:// or wss:// scheme matching the client base URL.
func (c *Client) ExecWebSocketURL(ctx context.Context, endpointID int, execID string) (wsURL string, token string, err error) {
	if endpointID <= 0 {
		return "", "", fmt.Errorf("portainer endpoint ID must be positive")
	}
	if err := validatePortainerExecID(execID); err != nil {
		return "", "", err
	}
	// Portainer requires a JWT token even when the primary auth mode is API key.
	// If apiKey auth is configured, we must obtain a JWT by authenticating first.
	if c.apiKey != "" {
		// For API-key-only clients we cannot obtain a JWT; the caller must
		// use username/password credentials for exec WebSocket sessions.
		// Return a clear error rather than silently failing.
		return "", "", fmt.Errorf("portainer exec websocket requires JWT auth; configure username/password credentials for exec sessions")
	}
	jwt, err := c.getJWT(ctx)
	if err != nil {
		return "", "", fmt.Errorf("acquire JWT for exec websocket: %w", err)
	}

	base := strings.TrimRight(c.baseURL, "/")
	// Translate http(s):// to ws(s)://.
	var wsBase string
	switch {
	case strings.HasPrefix(base, "https://"):
		wsBase = "wss://" + strings.TrimPrefix(base, "https://")
	case strings.HasPrefix(base, "http://"):
		wsBase = "ws://" + strings.TrimPrefix(base, "http://")
	default:
		wsBase = "ws://" + base
	}

	// Current Portainer releases use `id`; older releases used `execId`.
	// Supplying both is safe because each handler ignores the unknown field and
	// preserves compatibility across the supported homelab upgrade path.
	wsURL = fmt.Sprintf("%s/api/websocket/exec?endpointId=%d&id=%s&execId=%s",
		wsBase,
		endpointID,
		url.QueryEscape(execID),
		url.QueryEscape(execID),
	)
	return wsURL, jwt, nil
}
