package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	maxMCPIdentifierBytes = 255
	maxMCPPathBytes       = 4096
	maxMCPCommandBytes    = 64 * 1024
	maxMCPTextBytes       = 256 * 1024
	maxMCPJSONBytes       = 1024 * 1024
	maxMCPCollectionItems = 2000
)

var errMCPDependencyUnavailable = errors.New("MCP tool dependency is unavailable")

func requireBoundedString(req mcp.CallToolRequest, name string, maxBytes int) (string, error) {
	value, err := req.RequireString(name)
	if err != nil {
		return "", fmt.Errorf("%s is required", name)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	if len(value) > maxBytes {
		return "", fmt.Errorf("%s exceeds %d byte limit", name, maxBytes)
	}
	if strings.IndexByte(value, 0) >= 0 {
		return "", fmt.Errorf("%s contains a NUL byte", name)
	}
	return value, nil
}

func requireAssetID(req mcp.CallToolRequest) (string, error) {
	assetID, err := requireBoundedString(req, "asset_id", maxMCPIdentifierBytes)
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(assetID, "/\\\r\n") {
		return "", errors.New("asset_id contains invalid characters")
	}
	return assetID, nil
}

func requireCommand(req mcp.CallToolRequest) (string, error) {
	command, err := requireBoundedString(req, "command", maxMCPCommandBytes)
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(command, "\r\n") {
		return "", errors.New("command must be a single line")
	}
	return command, nil
}

func requirePath(req mcp.CallToolRequest) (string, error) {
	return requireBoundedString(req, "path", maxMCPPathBytes)
}

func boundedToolText(value string) *mcp.CallToolResult {
	const suffix = "\n...output truncated by MCP response limit"
	bounded, truncated := truncateUTF8(value, maxMCPTextBytes)
	if truncated {
		bounded, _ = truncateUTF8(value, maxMCPTextBytes-len(suffix))
		bounded += suffix
	}
	return mcp.NewToolResultText(bounded)
}

func toolJSON(value any) *mcp.CallToolResult {
	data, err := marshalBoundedJSON(value, true)
	if err != nil {
		return mcp.NewToolResultError(err.Error())
	}
	return mcp.NewToolResultText(string(data))
}

func toolValue(value any) *mcp.CallToolResult {
	if text, ok := value.(string); ok {
		return boundedToolText(text)
	}
	return toolJSON(value)
}

func marshalBoundedJSON(value any, indent bool) ([]byte, error) {
	var (
		data []byte
		err  error
	)
	if indent {
		data, err = json.MarshalIndent(value, "", "  ")
	} else {
		data, err = json.Marshal(value)
	}
	if err != nil {
		return nil, errors.New("failed to encode MCP response")
	}
	if len(data) > maxMCPJSONBytes {
		return nil, fmt.Errorf("MCP response exceeds %d byte limit", maxMCPJSONBytes)
	}
	return data, nil
}

func truncateUTF8(value string, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value, false
	}
	data := []byte(value)
	data = data[:maxBytes]
	for len(data) > 0 && !utf8.Valid(data) {
		data = data[:len(data)-1]
	}
	return string(data), true
}

func validateCollectionSize(name string, count int) error {
	if count > maxMCPCollectionItems {
		return fmt.Errorf("%s contains %d items; MCP limit is %d", name, count, maxMCPCollectionItems)
	}
	return nil
}

func (d *Deps) checkMutation(ctx context.Context, tool, target string) error {
	if d == nil || d.AuthorizeMutation == nil {
		return errors.New("MCP mutation policy is unavailable")
	}
	return d.AuthorizeMutation(ctx, tool, target)
}

func (d *Deps) auditMutation(ctx context.Context, tool, target, decision, reason string, details map[string]any) {
	if d == nil || d.AuditMutation == nil {
		return
	}
	d.AuditMutation(ctx, tool, target, decision, reason, details)
}

func errorReason(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "maintenance"):
		return "maintenance_blocked"
	case strings.Contains(message, "rate limit"):
		return "rate_limited"
	case strings.Contains(message, "access denied"), strings.Contains(message, "forbidden"):
		return "asset_forbidden"
	case strings.Contains(message, "not connected"), strings.Contains(message, "offline"):
		return "asset_offline"
	case strings.Contains(message, "timed out"), strings.Contains(message, "deadline"):
		return "timed_out"
	case strings.Contains(message, "cancel"):
		return "canceled"
	case strings.Contains(message, "not found"):
		return "target_not_found"
	case strings.Contains(message, "rejected"), strings.Contains(message, "not accepted"):
		return "operation_rejected"
	case strings.Contains(message, "unavailable"), strings.Contains(message, "not configured"):
		return "dependency_unavailable"
	default:
		return "operation_failed"
	}
}
