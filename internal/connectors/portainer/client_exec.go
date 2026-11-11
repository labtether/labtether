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

// ExecAuth is how the Hub signs in to Portainer's exec WebSocket: the API key
// in X-API-Key for an API-key connection, or a JWT in Authorization for a
// password connection. Secret never goes into the URL.
type ExecAuth struct {
	Header string
	Secret string
}

const (
	execAuthHeaderAPIKey = "X-API-Key"
	execAuthHeaderBearer = "Authorization"
)

// HeaderValue is the value of the auth header for the WebSocket handshake.
func (a ExecAuth) HeaderValue() string {
	if a.Header == execAuthHeaderBearer {
		return "Bearer " + a.Secret
	}
	return a.Secret
}

// ExecWebSocketURL returns the WebSocket URL and the credentials for an exec
// session via Portainer's /api/websocket/exec endpoint. Portainer accepts the
// same API key there as on its REST API; password connections use a JWT.
// wsURL uses the ws:// or wss:// scheme matching the client base URL.
func (c *Client) ExecWebSocketURL(ctx context.Context, endpointID int, execID string) (wsURL string, auth ExecAuth, err error) {
	if endpointID <= 0 {
		return "", ExecAuth{}, fmt.Errorf("portainer endpoint ID must be positive")
	}
	if err := validatePortainerExecID(execID); err != nil {
		return "", ExecAuth{}, err
	}
	if c.apiKey != "" {
		auth = ExecAuth{Header: execAuthHeaderAPIKey, Secret: c.apiKey}
	} else {
		jwt, err := c.getJWT(ctx)
		if err != nil {
			return "", ExecAuth{}, fmt.Errorf("acquire JWT for exec websocket: %w", err)
		}
		auth = ExecAuth{Header: execAuthHeaderBearer, Secret: jwt}
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
	return wsURL, auth, nil
}
