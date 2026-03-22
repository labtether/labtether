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

type SlackAdapter struct{}

func (s *SlackAdapter) Type() string { return ChannelTypeSlack }

func (s *SlackAdapter) Send(ctx context.Context, config map[string]any, payload map[string]any) error {
	webhookURL, _ := config["webhook_url"].(string)
	if webhookURL == "" {
		return fmt.Errorf("slack config missing webhook_url")
	}

	title, _ := payload["title"].(string)
	text, _ := payload["text"].(string)
	if title == "" && text == "" {
		return fmt.Errorf("slack payload requires title or text")
	}

	blocks := []map[string]any{}
	if title != "" {
		blocks = append(blocks, map[string]any{
			"type": "header",
			"text": map[string]any{
				"type": "plain_text",
				"text": title,
			},
		})
	}
	if text != "" {
		blocks = append(blocks, map[string]any{
			"type": "section",
			"text": map[string]any{
				"type": "mrkdwn",
				"text": text,
			},
		})
	}

	slackMsg := map[string]any{
		"blocks": blocks,
	}
	if text != "" {
		slackMsg["text"] = strings.TrimSpace(title + " " + text)
	}

	body, err := json.Marshal(slackMsg)
	if err != nil {
		return fmt.Errorf("slack marshal: %w", err)
	}

	req, err := securityruntime.NewOutboundRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("slack create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := securityruntime.DoOutboundRequest(client, req)
	if err != nil {
		return fmt.Errorf("slack request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("slack returned status %d", resp.StatusCode)
	}

	return nil
}
