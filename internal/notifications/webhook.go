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

type WebhookAdapter struct{}

func (w *WebhookAdapter) Type() string { return ChannelTypeWebhook }

func (w *WebhookAdapter) Send(ctx context.Context, config map[string]any, payload map[string]any) error {
	urlRaw, ok := config["url"]
	if !ok {
		return fmt.Errorf("webhook config missing url")
	}
	url, ok := urlRaw.(string)
	if !ok || strings.TrimSpace(url) == "" {
		return fmt.Errorf("webhook config url is not a valid string")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhook marshal payload: %w", err)
	}

	req, err := securityruntime.NewOutboundRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if headers, ok := config["headers"].(map[string]any); ok {
		for k, v := range headers {
			if s, ok := v.(string); ok {
				req.Header.Set(k, s)
			}
		}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := securityruntime.DoOutboundRequest(client, req)
	if err != nil {
		return fmt.Errorf("webhook request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}
