package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/runtimesettings"
	"github.com/labtether/labtether/internal/servicehttp"
)

const (
	// TailscaleServeStatusRoute is the HTTP path for the Tailscale serve status endpoint.
	TailscaleServeStatusRoute = "/settings/tailscale/serve"

	// EnvTailscaleManaged is the environment variable that enables managed Tailscale support.
	EnvTailscaleManaged = "LABTETHER_TAILSCALE_MANAGED"
)

// Package-level vars are pointers-to-functions so tests can swap them out.
var (
	TailscaleLookPath      = exec.LookPath
	TailscaleRunner        = runTailscaleCommand
	TailscaleFallbackPaths = defaultTailscaleFallbackPaths
)

// TailscaleServeStatusResponse is the JSON payload returned by the
// Tailscale serve status endpoint.
type TailscaleServeStatusResponse struct {
	TailscaleInstalled    bool     `json:"tailscale_installed"`
	BackendState          string   `json:"backend_state,omitempty"`
	LoggedIn              bool     `json:"logged_in"`
	Tailnet               string   `json:"tailnet,omitempty"`
	DNSName               string   `json:"dns_name,omitempty"`
	TailscaleIPs          []string `json:"tailscale_ips,omitempty"`
	TSNetURL              string   `json:"tsnet_url,omitempty"`
	ServeStatus           string   `json:"serve_status"`
	ServeConfigured       bool     `json:"serve_configured"`
	ServeTarget           string   `json:"serve_target,omitempty"`
	SuggestedTarget       string   `json:"suggested_target,omitempty"`
	SuggestedCommand      string   `json:"suggested_command,omitempty"`
	RecommendationState   string   `json:"recommendation_state"`
	RecommendationMessage string   `json:"recommendation_message"`
	CanManage             bool     `json:"can_manage"`
	ManagementMode        string   `json:"management_mode"`
	DesiredMode           string   `json:"desired_mode"`
	DesiredModeSource     string   `json:"desired_mode_source"`
	DesiredTarget         string   `json:"desired_target,omitempty"`
	DesiredTargetSource   string   `json:"desired_target_source"`
	StatusNote            string   `json:"status_note,omitempty"`
}

// TailscaleStatusSnapshot carries the parsed output of `tailscale status --json`.
// Exported so that cmd/labtether/tls_tailscale.go can use the struct fields
// without reimplementing the JSON parsing.
type TailscaleStatusSnapshot struct {
	BackendState string
	Tailnet      string
	DNSName      string
	TailscaleIPs []string
	LoggedIn     bool
}

type tailscaleServeInspection struct {
	Configured bool
	Target     string
	Detail     string
}

type tailscaleServeActionRequest struct {
	Action string `json:"action"`
	Target string `json:"target,omitempty"`
}

type tailscaleDesiredSettings struct {
	Mode         string
	ModeSource   string
	Target       string
	TargetSource string
}

// HandleTailscaleServeStatus handles GET and POST /settings/tailscale/serve.
func (d *Deps) HandleTailscaleServeStatus(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != TailscaleServeStatusRoute {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		servicehttp.WriteJSON(w, http.StatusOK, d.InspectTailscaleServeStatus())
	case http.MethodPost:
		d.handleTailscaleServeStatusMutation(w, r)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// InspectTailscaleServeStatus returns the current Tailscale serve status.
// Exported so that cmd/labtether callers (hub_connection_resolver.go) can
// invoke it without reimplementing the inspection logic.
func (d *Deps) InspectTailscaleServeStatus() TailscaleServeStatusResponse {
	response := TailscaleServeStatusResponse{
		ServeStatus:           "not_installed",
		RecommendationState:   "recommended_not_available",
		RecommendationMessage: "Optional, but strongly recommended: install Tailscale on the Docker host for easy secure remote access.",
		CanManage:             EnvOrDefaultBool(EnvTailscaleManaged, false),
		ManagementMode:        "guided",
	}
	if response.CanManage {
		response.ManagementMode = "managed"
	}
	response.applyDesiredSettings(d.resolveTailscaleDesiredSettings())
	response.SuggestedTarget = response.resolvedServeTarget(d.defaultTailscaleServeTarget())
	if response.DesiredMode != "serve" {
		response.applyDesiredRecommendation()
	}

	path, err := d.resolveTailscaleBinaryPath()
	if err != nil || strings.TrimSpace(path) == "" {
		return response
	}
	response.TailscaleInstalled = true

	statusOut, statusErr := d.runTailscale(4*time.Second, path, "status", "--json")
	if statusErr != nil {
		response.ServeStatus = "status_unavailable"
		response.RecommendationState = "recommended_login_required"
		response.RecommendationMessage = "Tailscale is installed, but LabTether could not read its status. Finish Tailscale login on the host, then verify remote access here."
		response.StatusNote = TrimTailscaleOutput(statusOut, statusErr)
		return response
	}

	status := ParseTailscaleStatusSnapshot(statusOut)
	response.BackendState = status.BackendState
	response.LoggedIn = status.LoggedIn
	response.Tailnet = status.Tailnet
	response.DNSName = status.DNSName
	response.TailscaleIPs = status.TailscaleIPs
	response.TSNetURL = buildTailscaleHTTPSURL(status.DNSName)
	if response.TailscaleInstalled && response.LoggedIn && response.SuggestedTarget != "" {
		response.SuggestedCommand = fmt.Sprintf("tailscale serve --bg %s", response.SuggestedTarget)
	}

	if !response.LoggedIn {
		response.ServeStatus = "login_required"
		if response.DesiredMode == "serve" {
			response.RecommendationState = "recommended_login_required"
			response.RecommendationMessage = "Optional, but strongly recommended: connect this host to Tailscale to unlock easy HTTPS remote access."
		}
		return response
	}

	serveInspection := d.inspectTailscaleServe(path)
	response.ServeConfigured = serveInspection.Configured
	response.ServeTarget = serveInspection.Target
	response.StatusNote = serveInspection.Detail
	if serveInspection.Configured {
		response.ServeStatus = "configured"
		response.RecommendationState = "enabled"
		response.RecommendationMessage = "Tailscale HTTPS is active for this host."
		return response
	}

	response.ServeStatus = "off"
	if response.DesiredMode == "serve" {
		response.RecommendationState = "recommended_disabled"
		response.RecommendationMessage = "Optional, but strongly recommended: enable Tailscale HTTPS for the easiest secure remote access, or keep your current setup and enable it later."
	} else {
		response.applyDesiredRecommendation()
	}
	return response
}

func (d *Deps) defaultTailscaleServeTarget() string {
	// Remote access should land operators on the console, not the raw hub API.
	// Advanced/local-dev deployments can still override this via runtime settings.
	return "http://127.0.0.1:3000"
}

func (d *Deps) handleTailscaleServeStatusMutation(w http.ResponseWriter, r *http.Request) {
	role := ""
	if d.UserRoleFromContext != nil {
		role = d.UserRoleFromContext(r.Context())
	}
	if !auth.HasAdminPrivileges(role) {
		servicehttp.WriteError(w, http.StatusForbidden, "forbidden")
		return
	}

	current := d.InspectTailscaleServeStatus()
	if !current.CanManage {
		servicehttp.WriteError(w, http.StatusConflict, "tailscale host management is not enabled for this deployment")
		return
	}

	var req tailscaleServeActionRequest
	if err := d.decodeJSONBody(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid tailscale serve request")
		return
	}

	path, err := d.resolveTailscaleBinaryPath()
	if err != nil || strings.TrimSpace(path) == "" {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "tailscale is not installed on the host")
		return
	}

	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "apply":
		target := strings.TrimSpace(req.Target)
		if target == "" {
			target = current.resolvedServeTarget(d.defaultTailscaleServeTarget())
		}
		if target == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "tailscale serve target is required")
			return
		}
		if definition, ok := runtimesettings.DefinitionByKey(runtimesettings.KeyRemoteAccessTailscaleTarget); ok {
			normalized, err := runtimesettings.NormalizeValue(definition, target)
			if err != nil {
				servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			target = normalized
		}
		if output, err := d.runTailscale(10*time.Second, path, "serve", "--bg", "--yes", target); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, TrimTailscaleOutput(output, err))
			return
		}
	case "disable":
		if output, err := d.runTailscale(10*time.Second, path, "serve", "reset"); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, TrimTailscaleOutput(output, err))
			return
		}
	default:
		servicehttp.WriteError(w, http.StatusBadRequest, "action must be apply or disable")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, d.InspectTailscaleServeStatus())
}

// resolveTailscaleBinaryPath uses the Deps function fields when set, otherwise
// falls back to the package-level vars.
func (d *Deps) resolveTailscaleBinaryPath() (string, error) {
	lp := TailscaleLookPath
	if d.TailscaleLookPath != nil {
		lp = d.TailscaleLookPath
	}
	fp := TailscaleFallbackPaths
	if d.TailscaleFallbackPaths != nil {
		fp = d.TailscaleFallbackPaths
	}
	return ResolveTailscaleBinaryPathWith(lp, fp)
}

// runTailscale uses the Deps TailscaleRunnerOverride when set, otherwise
// uses the package-level TailscaleRunner var.
func (d *Deps) runTailscale(timeout time.Duration, path string, args ...string) ([]byte, error) {
	if d.TailscaleRunnerOverride != nil {
		return d.TailscaleRunnerOverride(timeout, path, args...)
	}
	return TailscaleRunner(timeout, path, args...)
}

// inspectTailscaleServe calls the unexported inspection function using the
// Deps's runner and path resolver.
func (d *Deps) inspectTailscaleServe(path string) tailscaleServeInspection {
	jsonOut, jsonErr := d.runTailscale(4*time.Second, path, "serve", "status", "--json")
	if jsonErr == nil {
		if inspection, ok := parseTailscaleServeStatusJSON(jsonOut); ok {
			return inspection
		}
	}

	textOut, textErr := d.runTailscale(4*time.Second, path, "serve", "status")
	trimmed := strings.TrimSpace(string(textOut))
	if looksLikeNoServeConfig(trimmed) {
		return tailscaleServeInspection{Configured: false}
	}
	if textErr != nil {
		return tailscaleServeInspection{Configured: false, Detail: TrimTailscaleOutput(textOut, textErr)}
	}
	if trimmed == "" {
		return tailscaleServeInspection{Configured: false}
	}
	return parseTailscaleServeStatusText(trimmed)
}

func (d *Deps) resolveTailscaleDesiredSettings() tailscaleDesiredSettings {
	modeDefinition, _ := runtimesettings.DefinitionByKey(runtimesettings.KeyRemoteAccessMode)
	targetDefinition, _ := runtimesettings.DefinitionByKey(runtimesettings.KeyRemoteAccessTailscaleTarget)

	overrides := map[string]string{}
	if d.RuntimeStore != nil {
		listed, err := d.RuntimeStore.ListRuntimeSettingOverrides()
		if err == nil {
			overrides = listed
		}
	}

	modeValue, modeSource := ResolveRuntimeSettingValueForDefinition(modeDefinition, overrides)
	targetValue, targetSource := ResolveRuntimeSettingValueForDefinition(targetDefinition, overrides)
	return tailscaleDesiredSettings{
		Mode:         modeValue,
		ModeSource:   string(modeSource),
		Target:       targetValue,
		TargetSource: string(targetSource),
	}
}

func (r *TailscaleServeStatusResponse) applyDesiredSettings(settings tailscaleDesiredSettings) {
	r.DesiredMode = settings.Mode
	r.DesiredModeSource = settings.ModeSource
	r.DesiredTarget = strings.TrimSpace(settings.Target)
	r.DesiredTargetSource = settings.TargetSource
}

func (r TailscaleServeStatusResponse) resolvedServeTarget(fallback string) string {
	if strings.TrimSpace(r.DesiredTarget) != "" {
		return strings.TrimSpace(r.DesiredTarget)
	}
	return fallback
}

func (r *TailscaleServeStatusResponse) applyDesiredRecommendation() {
	switch strings.TrimSpace(r.DesiredMode) {
	case "off":
		r.RecommendationState = "disabled_by_choice"
		r.RecommendationMessage = "Tailscale HTTPS is turned off in LabTether settings. You can enable it later whenever you want the recommended remote-access path."
	case "manual":
		r.RecommendationState = "manual"
		r.RecommendationMessage = "LabTether is set to manual remote access. Use your own reverse proxy or TLS path, or switch back to Tailscale HTTPS later."
	default:
		r.RecommendationState = "recommended_disabled"
		r.RecommendationMessage = "Optional, but strongly recommended: enable Tailscale HTTPS for the easiest secure remote access, or keep your current setup and enable it later."
	}
}

// ResolveRuntimeSettingValueForDefinition resolves the effective value and
// source for a runtime setting definition given the current env and overrides.
// Exported so that cmd/labtether callers that reference it by name keep
// compiling after the function moved from cmd/labtether.
func ResolveRuntimeSettingValueForDefinition(definition runtimesettings.Definition, overrides map[string]string) (string, runtimesettings.Source) {
	envValue := runtimesettings.ResolveEnvValue(definition, os.Getenv)
	overrideValue := ""
	if rawOverride, ok := overrides[definition.Key]; ok {
		if normalized, err := runtimesettings.NormalizeValue(definition, rawOverride); err == nil {
			overrideValue = normalized
		}
	}
	return runtimesettings.EffectiveValue(definition, envValue, overrideValue)
}

// EnvOrDefaultBool reads an environment variable as a bool, returning fallback
// when unset or unparseable. Exported for use in cmd/labtether callers that
// referenced the old package-level envOrDefaultBool.
func EnvOrDefaultBool(key string, fallback bool) bool {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	switch strings.ToLower(val) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	}
	return fallback
}

// TrimTailscaleOutput extracts a brief error string from raw tailscale command
// output. Exported so cmd/labtether test helpers can reuse it.
func TrimTailscaleOutput(output []byte, err error) string {
	parts := make([]string, 0, 2)
	if trimmed := strings.TrimSpace(string(output)); trimmed != "" {
		parts = append(parts, trimmed)
	}
	if err != nil {
		parts = append(parts, err.Error())
	}
	if len(parts) == 0 {
		return ""
	}
	joined := strings.Join(parts, " ")
	joined = strings.Join(strings.Fields(joined), " ")
	if len(joined) > 220 {
		return joined[:220]
	}
	return joined
}

func runTailscaleCommand(timeout time.Duration, path string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, args...) // #nosec G204 -- Path and args are built from validated tailscale CLI invocations in this package.
	return cmd.CombinedOutput()
}

// ResolveTailscaleBinaryPath locates the tailscale binary using PATH lookup
// and platform-specific fallback paths. It uses the package-level vars
// TailscaleLookPath and TailscaleFallbackPaths. Exported for tls_tailscale.go
// via the admin_bridge.go alias.
func ResolveTailscaleBinaryPath() (string, error) {
	return ResolveTailscaleBinaryPathWith(TailscaleLookPath, TailscaleFallbackPaths)
}

// ResolveTailscaleBinaryPathWith is the inner implementation used by both
// ResolveTailscaleBinaryPath and the bridge alias, allowing callers to supply
// their own lookup and fallback functions.
func ResolveTailscaleBinaryPathWith(lookPath func(string) (string, error), fallbackPaths func() []string) (string, error) {
	if path, err := lookPath("tailscale"); err == nil && strings.TrimSpace(path) != "" {
		return path, nil
	}

	for _, candidate := range fallbackPaths() {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		return candidate, nil
	}

	return "", fmt.Errorf("tailscale binary not found")
}

func defaultTailscaleFallbackPaths() []string {
	if runtime.GOOS != "darwin" {
		return nil
	}
	return []string{
		filepath.Join("/Applications", "Tailscale.app", "Contents", "MacOS", "Tailscale"),
		filepath.Join(os.Getenv("HOME"), "Applications", "Tailscale.app", "Contents", "MacOS", "Tailscale"),
	}
}

// ParseTailscaleStatusSnapshot decodes the JSON output of `tailscale status --json`
// into a TailscaleStatusSnapshot. Exported for use in tls_tailscale.go via the
// admin_bridge.go alias.
func ParseTailscaleStatusSnapshot(raw []byte) TailscaleStatusSnapshot {
	var status struct {
		BackendState   string `json:"BackendState"`
		CurrentTailnet struct {
			Name           string `json:"Name"`
			MagicDNSSuffix string `json:"MagicDNSSuffix"`
		} `json:"CurrentTailnet"`
		Self struct {
			DNSName      string   `json:"DNSName"`
			TailscaleIPs []string `json:"TailscaleIPs"`
		} `json:"Self"`
	}
	if err := json.Unmarshal(raw, &status); err != nil {
		return TailscaleStatusSnapshot{}
	}

	tailnet := strings.TrimSpace(status.CurrentTailnet.Name)
	if tailnet == "" {
		tailnet = strings.TrimSuffix(strings.TrimSpace(status.CurrentTailnet.MagicDNSSuffix), ".")
	}
	dnsName := strings.TrimSuffix(strings.TrimSpace(status.Self.DNSName), ".")
	backendState := strings.TrimSpace(status.BackendState)
	loggedIn := dnsName != "" || tailnet != ""
	if !loggedIn {
		switch strings.ToLower(backendState) {
		case "running", "connected", "starting":
			loggedIn = true
		}
	}

	return TailscaleStatusSnapshot{
		BackendState: backendState,
		Tailnet:      tailnet,
		DNSName:      dnsName,
		TailscaleIPs: append([]string(nil), status.Self.TailscaleIPs...),
		LoggedIn:     loggedIn,
	}
}

func buildTailscaleHTTPSURL(dnsName string) string {
	trimmed := strings.TrimSuffix(strings.TrimSpace(dnsName), ".")
	if trimmed == "" {
		return ""
	}
	return "https://" + trimmed
}

func parseTailscaleServeStatusJSON(raw []byte) (tailscaleServeInspection, bool) {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return tailscaleServeInspection{}, false
	}

	target, configured := findFirstServeTarget(payload)
	if configured {
		return tailscaleServeInspection{
			Configured: true,
			Target:     target,
			Detail:     "Detected existing Tailscale Serve configuration.",
		}, true
	}
	if mapValue, ok := payload.(map[string]any); ok && len(mapValue) == 0 {
		return tailscaleServeInspection{Configured: false}, true
	}
	return tailscaleServeInspection{Configured: false}, true
}

func findFirstServeTarget(value any) (string, bool) {
	switch typed := value.(type) {
	case map[string]any:
		if len(typed) == 0 {
			return "", false
		}
		for _, candidateKey := range []string{"Target", "target", "Proxy", "proxy"} {
			if candidate, ok := typed[candidateKey]; ok {
				if target, found := findFirstServeTarget(candidate); found {
					return target, true
				}
			}
		}
		for _, candidate := range typed {
			if target, found := findFirstServeTarget(candidate); found {
				return target, true
			}
		}
		return "", true
	case []any:
		if len(typed) == 0 {
			return "", false
		}
		for _, candidate := range typed {
			if target, found := findFirstServeTarget(candidate); found {
				return target, true
			}
		}
		return "", true
	case string:
		trimmed := strings.TrimSpace(typed)
		if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
			return trimmed, true
		}
		return "", false
	default:
		return "", false
	}
}

func parseTailscaleServeStatusText(raw string) tailscaleServeInspection {
	lines := strings.Split(raw, "\n")
	firstLine := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if firstLine == "" {
			firstLine = trimmed
		}
		for _, field := range strings.Fields(trimmed) {
			if strings.HasPrefix(field, "http://") || strings.HasPrefix(field, "https://") {
				return tailscaleServeInspection{
					Configured: true,
					Target:     field,
					Detail:     firstLine,
				}
			}
		}
	}
	if firstLine == "" {
		return tailscaleServeInspection{Configured: false}
	}
	return tailscaleServeInspection{
		Configured: true,
		Detail:     firstLine,
	}
}

func looksLikeNoServeConfig(raw string) bool {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return true
	}
	return strings.Contains(normalized, "no serve config") ||
		strings.Contains(normalized, "nothing is being served")
}
