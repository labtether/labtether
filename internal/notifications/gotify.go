package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/securityruntime"
)

type GotifyAdapter struct{}

func (g *GotifyAdapter) Type() string { return ChannelTypeGotify }

func (g *GotifyAdapter) Send(ctx context.Context, config map[string]any, payload map[string]any) error {
	serverURL := strings.TrimSpace(payloadString(config, "server_url"))
	if serverURL == "" {
		return fmt.Errorf("gotify config missing server_url")
	}
	appToken := firstNonBlank(payloadString(config, "app_token"), payloadString(config, "token"))
	if strings.TrimSpace(appToken) == "" {
		return fmt.Errorf("gotify config missing app_token")
	}

	title := firstNonBlank(payloadString(payload, "title"), "LabTether Alert")
	message := firstNonBlank(payloadString(payload, "text"), payloadString(payload, "message"))
	if message == "" {
		return fmt.Errorf("gotify payload requires text or message")
	}
	priority, _ := notificationPriority(config["priority"])

	body, err := json.Marshal(map[string]any{
		"title":    title,
		"message":  message,
		"priority": priority,
	})
	if err != nil {
		return fmt.Errorf("gotify marshal payload: %w", err)
	}

	url := strings.TrimRight(serverURL, "/") + "/message"
	req, err := securityruntime.NewOutboundRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("gotify create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gotify-Key", appToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := securityruntime.DoOutboundRequest(client, req)
	if err != nil {
		return fmt.Errorf("gotify request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("gotify returned status %d", resp.StatusCode)
	}
	return nil
}
