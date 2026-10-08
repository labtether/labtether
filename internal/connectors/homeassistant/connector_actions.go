package homeassistant

import (
	"context"
	"fmt"
	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/connectorsdk"
	"net/http"
	"net/url"
	"strings"
)

func (c *Connector) Actions() []connectorsdk.ActionDescriptor {
	return []connectorsdk.ActionDescriptor{
		{
			ID:             "entity.toggle",
			Name:           "Toggle Entity",
			Description:    "Toggle an entity state.",
			RequiresTarget: true,
			SupportsDryRun: true,
		},
		{
			ID:             "service.call",
			Name:           "Call Service",
			Description:    "Invoke a Home Assistant domain service.",
			RequiresTarget: false,
			SupportsDryRun: true,
			Parameters: []connectorsdk.ActionParameter{
				{
					Key:         "service",
					Label:       "Service",
					Required:    true,
					Description: "Service key, for example light.turn_on.",
				},
			},
		},
	}
}

func (c *Connector) ExecuteAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	if !c.isConfigured() {
		return connectorsdk.ActionResult{
			Status:  "failed",
			Message: "home assistant connector is not configured; actions are unavailable",
		}, nil
	}

	target := assetid.NativeCollectorAssetID(req.TargetID)
	if req.DryRun {
		switch actionID {
		case "entity.toggle":
			if target == "" {
				return connectorsdk.ActionResult{Status: "failed", Message: "target_id is required"}, nil
			}
			return connectorsdk.ActionResult{Status: "succeeded", Message: "dry-run: toggle validated", Output: fmt.Sprintf("would toggle %s", target)}, nil
		case "service.call":
			service := strings.TrimSpace(req.Params["service"])
			if service == "" {
				return connectorsdk.ActionResult{Status: "failed", Message: "service is required"}, nil
			}
			return connectorsdk.ActionResult{Status: "succeeded", Message: "dry-run: service call validated", Output: fmt.Sprintf("would call %s", service)}, nil
		default:
			return connectorsdk.ActionResult{Status: "failed", Message: "unsupported action"}, nil
		}
	}

	switch actionID {
	case "entity.toggle":
		if target == "" {
			return connectorsdk.ActionResult{Status: "failed", Message: "target_id is required"}, nil
		}
		body := map[string]any{"entity_id": target}
		payload, err := c.request(ctx, http.MethodPost, "/api/services/homeassistant/toggle", body)
		if err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
		return connectorsdk.ActionResult{Status: "succeeded", Message: "entity toggled", Output: truncatePayloadRedacted(payload, c.token)}, nil
	case "service.call":
		service := strings.TrimSpace(req.Params["service"])
		if service == "" {
			return connectorsdk.ActionResult{Status: "failed", Message: "service is required"}, nil
		}
		domain, action, err := parseService(service)
		if err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}

		body := map[string]any{}
		for key, value := range req.Params {
			if strings.TrimSpace(key) == "service" {
				continue
			}
			body[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
		if target != "" {
			body["entity_id"] = target
		}

		payload, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/services/%s/%s", url.PathEscape(domain), url.PathEscape(action)), body)
		if err != nil {
			return connectorsdk.ActionResult{Status: "failed", Message: err.Error()}, nil
		}
		return connectorsdk.ActionResult{Status: "succeeded", Message: "service called", Output: truncatePayloadRedacted(payload, c.token)}, nil
	default:
		return connectorsdk.ActionResult{Status: "failed", Message: "unsupported action"}, nil
	}
}

func parseService(value string) (string, string, error) {
	trimmed := strings.TrimSpace(value)
	parts := strings.Split(trimmed, ".")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("service must be domain.action")
	}
	domain := strings.TrimSpace(parts[0])
	action := strings.TrimSpace(parts[1])
	if domain == "" || action == "" {
		return "", "", fmt.Errorf("service must be domain.action")
	}
	if !validServicePathPart(domain) || !validServicePathPart(action) {
		return "", "", fmt.Errorf("service domain/action may contain only lowercase letters, digits, and underscores")
	}
	return domain, action, nil
}

func validServicePathPart(value string) bool {
	for _, ch := range value {
		if ch >= 'a' && ch <= 'z' {
			continue
		}
		if ch >= '0' && ch <= '9' {
			continue
		}
		if ch == '_' {
			continue
		}
		return false
	}
	return value != ""
}
