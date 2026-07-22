package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/labtether/labtether/internal/apikeys"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/auth"
	dockerconnector "github.com/labtether/labtether/internal/connectors/docker"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

const (
	mcpInternalJSONLimit = 1024 * 1024
	mcpInternalFileLimit = 256 * 1024
)

// boundedResponseRecorder prevents an internal handler bridge from buffering an
// arbitrarily large agent or store response in the hub process.
type boundedResponseRecorder struct {
	header   http.Header
	status   int
	body     bytes.Buffer
	limit    int
	overflow bool
}

func newBoundedResponseRecorder(limit int) *boundedResponseRecorder {
	return &boundedResponseRecorder{header: make(http.Header), limit: limit}
}

func (r *boundedResponseRecorder) Header() http.Header { return r.header }

func (r *boundedResponseRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *boundedResponseRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	if r.limit <= 0 || r.body.Len()+len(p) <= r.limit {
		_, _ = r.body.Write(p)
		return len(p), nil
	}
	remaining := r.limit - r.body.Len()
	if remaining > 0 {
		_, _ = r.body.Write(p[:remaining])
	}
	r.overflow = true
	return len(p), nil
}

func invokeMCPHandler(ctx context.Context, method, target string, body []byte, limit int, handler http.HandlerFunc) ([]byte, http.Header, error) {
	if handler == nil {
		return nil, nil, errors.New("MCP dependency is unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, nil, errors.New("failed to prepare internal MCP request")
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	recorder := newBoundedResponseRecorder(limit)
	handler(recorder, req)
	if recorder.overflow {
		return nil, recorder.header, errors.New("MCP dependency response exceeded size limit")
	}
	status := recorder.status
	if status == 0 {
		status = http.StatusOK
	}
	if status < 200 || status >= 300 {
		message := "MCP dependency request failed"
		var payload map[string]any
		if json.Unmarshal(recorder.body.Bytes(), &payload) == nil {
			if value, ok := payload["error"].(string); ok && strings.TrimSpace(value) != "" {
				message = value
			}
			if value, ok := payload["message"].(string); ok && strings.TrimSpace(value) != "" {
				message = value
			}
		}
		return nil, recorder.header, fmt.Errorf("%s (status %d)", shared.SanitizeUpstreamError(message), status)
	}
	return append([]byte(nil), recorder.body.Bytes()...), recorder.header, nil
}

func decodeMCPJSON(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, errors.New("MCP dependency returned invalid JSON")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("MCP dependency returned multiple JSON values")
	}
	return value, nil
}

func safeMCPConnectorMessage(message string) string {
	if strings.TrimSpace(message) == "" {
		return ""
	}
	message = shared.SanitizeUpstreamError(message)
	if len(message) <= 512 {
		return message
	}
	message = message[:512]
	for !utf8.ValidString(message) {
		message = message[:len(message)-1]
	}
	return message
}

func (s *apiServer) mcpJSONHandler(method, target string, body []byte, handler http.HandlerFunc) func(context.Context) (any, error) {
	return func(ctx context.Context) (any, error) {
		data, _, err := invokeMCPHandler(ctx, method, target, body, mcpInternalJSONLimit, handler)
		if err != nil {
			return nil, err
		}
		return decodeMCPJSON(data)
	}
}

func (s *apiServer) mcpListServices() func(context.Context, string) (any, error) {
	return func(ctx context.Context, assetID string) (any, error) {
		deps := s.buildResourcesDeps()
		return s.mcpJSONHandler(http.MethodGet, "/services/"+url.PathEscape(assetID), nil, deps.HandleServices)(ctx)
	}
}

func (s *apiServer) mcpRestartService() func(context.Context, string, string) (any, error) {
	return func(ctx context.Context, assetID, serviceName string) (any, error) {
		body, err := json.Marshal(map[string]string{"service": serviceName})
		if err != nil {
			return nil, errors.New("failed to encode service request")
		}
		deps := s.buildResourcesDeps()
		return s.mcpJSONHandler(http.MethodPost, "/services/"+url.PathEscape(assetID)+"/restart", body, deps.HandleServices)(ctx)
	}
}

func (s *apiServer) mcpListFiles() func(context.Context, string, string) (any, error) {
	return func(ctx context.Context, assetID, path string) (any, error) {
		query := url.Values{"path": []string{path}}.Encode()
		deps := s.buildResourcesDeps()
		return s.mcpJSONHandler(http.MethodGet, "/files/"+url.PathEscape(assetID)+"/list?"+query, nil, deps.HandleFiles)(ctx)
	}
}

func (s *apiServer) mcpReadFile() func(context.Context, string, string) (any, error) {
	return func(ctx context.Context, assetID, path string) (any, error) {
		query := url.Values{"path": []string{path}}.Encode()
		deps := s.buildResourcesDeps()
		handler := func(w http.ResponseWriter, r *http.Request) {
			deps.HandleFileDownloadWithLimit(w, r, assetID, 30*time.Second, mcpInternalFileLimit)
		}
		data, headers, err := invokeMCPHandler(ctx, http.MethodGet, "/files/"+url.PathEscape(assetID)+"/download?"+query, nil, mcpInternalFileLimit, handler)
		if err != nil {
			return nil, err
		}
		result := map[string]any{"path": path, "size": len(data)}
		if utf8.Valid(data) {
			result["content"] = string(data)
			result["encoding"] = "utf-8"
		} else {
			result["content"] = base64.StdEncoding.EncodeToString(data)
			result["encoding"] = "base64"
		}
		if contentType := strings.TrimSpace(headers.Get("Content-Type")); contentType != "" {
			result["content_type"] = contentType
		}
		return result, nil
	}
}

func (s *apiServer) mcpListProcesses() func(context.Context, string) (any, error) {
	return func(ctx context.Context, assetID string) (any, error) {
		deps := s.buildResourcesDeps()
		target := "/processes/" + url.PathEscape(assetID) + "?sort=cpu&limit=50"
		return s.mcpJSONHandler(http.MethodGet, target, nil, deps.HandleProcesses)(ctx)
	}
}

func (s *apiServer) mcpListNetwork() func(context.Context, string) (any, error) {
	return func(ctx context.Context, assetID string) (any, error) {
		deps := s.buildResourcesDeps()
		return s.mcpJSONHandler(http.MethodGet, "/network/"+url.PathEscape(assetID), nil, deps.HandleNetworks)(ctx)
	}
}

func (s *apiServer) mcpListDisks() func(context.Context, string) (any, error) {
	return func(ctx context.Context, assetID string) (any, error) {
		deps := s.buildResourcesDeps()
		return s.mcpJSONHandler(http.MethodGet, "/disks/"+url.PathEscape(assetID), nil, deps.HandleDisks)(ctx)
	}
}

func (s *apiServer) mcpListPackages() func(context.Context, string) (any, error) {
	return func(ctx context.Context, assetID string) (any, error) {
		deps := s.buildResourcesDeps()
		return s.mcpJSONHandler(http.MethodGet, "/packages/"+url.PathEscape(assetID), nil, deps.HandlePackages)(ctx)
	}
}

func (s *apiServer) mcpMetricsOverview() func(context.Context) (map[string]any, error) {
	return func(ctx context.Context) (map[string]any, error) {
		deps := s.buildResourcesDeps()
		value, err := s.mcpJSONHandler(http.MethodGet, "/metrics/overview", nil, deps.HandleMetricsOverview)(ctx)
		if err != nil {
			return nil, err
		}
		result, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("metrics dependency returned an invalid response")
		}
		return result, nil
	}
}

func mcpRateKey(actor, tool, target string) string {
	sum := sha256.Sum256([]byte(actor + "\x00" + tool + "\x00" + target))
	return "mcp:" + hex.EncodeToString(sum[:])
}

func mcpMutationRate(tool string) int {
	switch tool {
	case "asset_reboot", "asset_shutdown":
		return 6
	case "alerts_acknowledge":
		return 120
	case "exec", "exec_multi", "services_restart", "docker_container_restart":
		return 30
	default:
		return 20
	}
}

func (s *apiServer) mcpAuthorizeMutation() func(context.Context, string, string) error {
	return func(ctx context.Context, tool, target string) error {
		actor := strings.TrimSpace(userIDFromContext(ctx))
		if actor == "" {
			return errors.New("MCP principal is unavailable")
		}
		if !auth.HasWritePrivileges(userRoleFromContext(ctx)) {
			return errors.New("MCP mutations require operator role")
		}

		assetTarget := strings.TrimSpace(target)
		allowedAssets := allowedAssetsFromContext(ctx)
		if tool == "docker_container_restart" {
			resolved, err := s.mcpResolveDockerContainer("", target)
			if err != nil {
				if len(allowedAssets) > 0 {
					return errors.New("access denied: target is unavailable or outside allowed assets")
				}
				return err
			}
			if !apikeys.AssetAllowed(allowedAssets, resolved.host.AgentID) {
				return errors.New("access denied: target is unavailable or outside allowed assets")
			}
			assetTarget = resolved.host.AgentID
		} else if tool != "alerts_acknowledge" && !apikeys.AssetAllowed(allowedAssets, assetTarget) {
			return errors.New("access denied: target is unavailable or outside allowed assets")
		}

		// Wake-on-LAN is executed through the production HTTP handler, which owns
		// its maintenance-window and rate-limit checks. The MCP policy still owns
		// the principal and role decision so read-only MCP clients cannot dispatch.
		if tool == "asset_wake" {
			return nil
		}
		if tool != "alerts_acknowledge" {
			guardrails, err := s.ensureGroupFeaturesDeps().EvaluateAssetGuardrails(assetTarget, time.Now().UTC())
			if err != nil {
				return errors.New("failed to evaluate maintenance windows")
			}
			if guardrails.BlockActions {
				return errors.New("actions are blocked by active maintenance windows")
			}
		}
		recorder := newBoundedResponseRecorder(1024)
		if !s.enforceRateLimitGlobal(recorder, mcpRateKey(actor, tool, assetTarget), mcpMutationRate(tool), time.Minute) {
			return errors.New("MCP mutation rate limit exceeded")
		}
		return nil
	}
}

func redactedMCPAuditDetails(tool string, details map[string]any) map[string]any {
	out := map[string]any{"tool": tool}
	for _, key := range []string{"action", "service", "command_bytes", "exit_code", "truncated"} {
		if value, ok := details[key]; ok {
			out[key] = value
		}
	}
	return out
}

func (s *apiServer) mcpAuditMutation() func(context.Context, string, string, string, string, map[string]any) {
	return func(ctx context.Context, tool, target, decision, reason string, details map[string]any) {
		s.appendAuditEventBestEffort(audit.Event{
			Type:      "mcp.tool",
			ActorID:   principalActorID(ctx),
			Target:    target,
			Decision:  decision,
			Reason:    reason,
			Details:   redactedMCPAuditDetails(tool, details),
			Timestamp: time.Now().UTC(),
		}, "api warning: failed to append MCP tool audit event")
	}
}

func (s *apiServer) mcpWakeAsset() func(context.Context, string) (map[string]any, error) {
	return func(ctx context.Context, assetID string) (map[string]any, error) {
		if strings.TrimSpace(userIDFromContext(ctx)) == "" {
			return nil, errors.New("MCP principal is unavailable")
		}
		deps := s.buildResourcesDeps()
		deps.EnforceRateLimit = func(w http.ResponseWriter, r *http.Request, bucket string, limit int, window time.Duration) bool {
			return s.enforceRateLimitGlobal(w, mcpRateKey(principalActorID(r.Context()), bucket, assetID), limit, window)
		}
		handler := func(w http.ResponseWriter, r *http.Request) { deps.HandleWakeOnLAN(w, r, assetID) }
		value, err := s.mcpJSONHandler(http.MethodPost, "/assets/"+url.PathEscape(assetID)+"/wake", nil, handler)(ctx)
		if err != nil {
			return nil, err
		}
		result, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("wake dependency returned an invalid response")
		}
		return result, nil
	}
}

type mcpDockerContainer struct {
	host      *dockerconnector.DockerHost
	container *dockerconnector.ContainerState
	canonical string
}

func dockerContainerCanonicalID(hostID, containerID string) string {
	shortID := containerID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	hostKey := strings.ToLower(strings.TrimSpace(hostID))
	hostKey = strings.ReplaceAll(hostKey, " ", "-")
	hostKey = strings.ReplaceAll(hostKey, ".", "-")
	return "docker-ct-" + hostKey + "-" + shortID
}

func dockerHostMatches(hostID, requested string) bool {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return true
	}
	canonicalHost := strings.TrimPrefix(requested, "docker-host-")
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(hostID, " ", "-"), ".", "-"))
	return strings.EqualFold(requested, hostID) || strings.EqualFold(canonicalHost, normalized)
}

func (s *apiServer) mcpResolveDockerContainer(hostID, ref string) (mcpDockerContainer, error) {
	if s.dockerCoordinator == nil {
		return mcpDockerContainer{}, errors.New("docker coordinator is unavailable")
	}
	if host, container, ok := s.dockerCoordinator.FindContainer(ref); ok {
		if !dockerHostMatches(host.AgentID, hostID) {
			return mcpDockerContainer{}, errors.New("container does not belong to requested Docker host")
		}
		return mcpDockerContainer{host: host, container: container, canonical: ref}, nil
	}
	var matches []mcpDockerContainer
	for _, host := range s.dockerCoordinator.ListHosts() {
		if host == nil || !dockerHostMatches(host.AgentID, hostID) {
			continue
		}
		for index := range host.Containers {
			container := host.Containers[index]
			canonical := dockerContainerCanonicalID(host.AgentID, container.ID)
			shortID := container.ID
			if len(shortID) > 12 {
				shortID = shortID[:12]
			}
			if ref == canonical || ref == container.ID || ref == shortID || ref == container.Name {
				containerCopy := container
				matches = append(matches, mcpDockerContainer{host: host, container: &containerCopy, canonical: canonical})
			}
		}
	}
	if len(matches) == 0 {
		return mcpDockerContainer{}, fmt.Errorf("container not found: %s", ref)
	}
	if len(matches) > 1 {
		return mcpDockerContainer{}, fmt.Errorf("container reference is ambiguous: %s", ref)
	}
	return matches[0], nil
}

func (s *apiServer) mcpDockerContainerLogs() func(context.Context, string, string, int) (string, error) {
	if s.dockerCoordinator == nil {
		return nil
	}
	return func(ctx context.Context, assetID, containerID string, tail int) (string, error) {
		resolved, err := s.mcpResolveDockerContainer(assetID, containerID)
		if err != nil {
			return "", err
		}
		if !dockerHostMatches(resolved.host.AgentID, assetID) {
			return "", errors.New("container does not belong to requested Docker host")
		}
		result, err := s.dockerCoordinator.ExecuteAction(ctx, "container.logs", connectorsdk.ActionRequest{
			TargetID: resolved.canonical,
			Params:   map[string]string{"tail": strconv.Itoa(tail)},
		})
		if err != nil {
			return "", errors.New("Docker log request failed")
		}
		if !strings.EqualFold(strings.TrimSpace(result.Status), "succeeded") {
			return "", errors.New("Docker log request failed")
		}
		return result.Output, nil
	}
}

func (s *apiServer) mcpDockerContainerStats() func(context.Context, string, string) (map[string]any, error) {
	if s.dockerCoordinator == nil {
		return nil
	}
	return func(ctx context.Context, assetID, containerID string) (map[string]any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		resolved, err := s.mcpResolveDockerContainer(assetID, containerID)
		if err != nil {
			return nil, err
		}
		stats, ok := s.dockerCoordinator.GetContainerStats(resolved.host.AgentID, resolved.container.ID)
		if !ok {
			return map[string]any{"available": false, "container_id": resolved.canonical}, nil
		}
		encoded, err := json.Marshal(stats)
		if err != nil {
			return nil, errors.New("failed to encode Docker stats")
		}
		var result map[string]any
		if err := json.Unmarshal(encoded, &result); err != nil {
			return nil, errors.New("failed to encode Docker stats")
		}
		result["available"] = true
		result["container_id"] = resolved.canonical
		return result, nil
	}
}
