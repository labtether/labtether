package notifications

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/securityruntime"
)

type NtfyAdapter struct{}

func (n *NtfyAdapter) Type() string { return ChannelTypeNtfy }

func (n *NtfyAdapter) Send(ctx context.Context, config map[string]any, payload map[string]any) error {
	serverURL := strings.TrimSpace(payloadString(config, "server_url"))
	if serverURL == "" {
		return fmt.Errorf("ntfy config missing server_url")
	}
	topic := strings.Trim(strings.TrimSpace(payloadString(config, "topic")), "/")
	if topic == "" {
		return fmt.Errorf("ntfy config missing topic")
	}

	title := payloadString(payload, "title")
	text := firstNonBlank(payloadString(payload, "text"), payloadString(payload, "message"), title)
	if text == "" {
		return fmt.Errorf("ntfy payload requires title or text")
	}

	url := strings.TrimRight(serverURL, "/") + "/" + topic
	req, err := securityruntime.NewOutboundRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(text))
	if err != nil {
		return fmt.Errorf("ntfy create request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	if title != "" {
		req.Header.Set("Title", title)
	}
	if clickURL := strings.TrimSpace(payloadString(config, "click")); clickURL != "" {
		req.Header.Set("Click", clickURL)
	}
	if priority, ok := notificationPriority(config["priority"]); ok {
		req.Header.Set("Priority", strconv.Itoa(priority))
	}
	if tags := strings.TrimSpace(payloadString(config, "tags")); tags != "" {
		req.Header.Set("Tags", tags)
	}
	if token := strings.TrimSpace(payloadString(config, "token")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if username := strings.TrimSpace(payloadString(config, "username")); username != "" {
		req.SetBasicAuth(username, payloadString(config, "password"))
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := securityruntime.DoOutboundRequest(client, req)
	if err != nil {
		return fmt.Errorf("ntfy request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("ntfy returned status %d", resp.StatusCode)
	}
	return nil
}

func notificationPriority(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return clampPriority(typed), true
	case int32:
		return clampPriority(int(typed)), true
	case int64:
		return clampPriority(int(typed)), true
	case float64:
		return clampPriority(int(typed)), true
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return 0, false
		}
		parsed, err := strconv.Atoi(trimmed)
		if err != nil {
			return 0, false
		}
		return clampPriority(parsed), true
	default:
		return 0, false
	}
}

func clampPriority(value int) int {
	if value < 1 {
		return 1
	}
	if value > 5 {
		return 5
	}
	return value
}
