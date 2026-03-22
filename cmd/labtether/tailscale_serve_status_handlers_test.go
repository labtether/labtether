package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHandleTailscaleServeStatus_NotInstalled(t *testing.T) {
	sut := newTestAPIServer(t)

	originalLookPath := tailscaleLookPath
	originalRunner := tailscaleRunner
	originalFallbackPaths := tailscaleFallbackPaths
	t.Cleanup(func() {
		tailscaleLookPath = originalLookPath
		tailscaleRunner = originalRunner
		tailscaleFallbackPaths = originalFallbackPaths
	})

	tailscaleLookPath = func(file string) (string, error) {
		if file != "tailscale" {
			t.Fatalf("unexpected executable lookup: %s", file)
		}
		return "", errors.New("not found")
	}
	tailscaleFallbackPaths = func() []string { return nil }
	tailscaleRunner = func(time.Duration, string, ...string) ([]byte, error) {
		t.Fatalf("tailscale CLI should not run when the binary is unavailable")
		return nil, nil
	}

	req := httptest.NewRequest(http.MethodGet, tailscaleServeStatusRoute, nil)
	rec := httptest.NewRecorder()
	sut.handleTailscaleServeStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response tailscaleServeStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.TailscaleInstalled {
		t.Fatalf("expected tailscale_installed=false")
	}
	if response.ServeStatus != "not_installed" {
		t.Fatalf("expected serve_status=not_installed, got %q", response.ServeStatus)
	}
	if response.DesiredMode != "serve" {
		t.Fatalf("expected desired_mode=serve by default, got %q", response.DesiredMode)
	}
	if response.RecommendationState != "recommended_not_available" {
		t.Fatalf("expected recommendation_not_available, got %q", response.RecommendationState)
	}
	if !strings.Contains(response.RecommendationMessage, "strongly recommended") {
		t.Fatalf("expected recommendation to mention strong recommendation, got %q", response.RecommendationMessage)
	}
}

func TestHandleTailscaleServeStatus_LoginRequired(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true

	originalLookPath := tailscaleLookPath
	originalRunner := tailscaleRunner
	originalFallbackPaths := tailscaleFallbackPaths
	t.Cleanup(func() {
		tailscaleLookPath = originalLookPath
		tailscaleRunner = originalRunner
		tailscaleFallbackPaths = originalFallbackPaths
	})

	tailscaleLookPath = func(file string) (string, error) {
		return "/usr/local/bin/tailscale", nil
	}
	tailscaleRunner = func(_ time.Duration, path string, args ...string) ([]byte, error) {
		if path != "/usr/local/bin/tailscale" {
			t.Fatalf("unexpected path %q", path)
		}
		if len(args) == 2 && args[0] == "status" && args[1] == "--json" {
			return []byte(`{
				"BackendState": "NeedsLogin",
				"CurrentTailnet": {},
				"Self": {}
			}`), nil
		}
		t.Fatalf("unexpected tailscale invocation: %v", args)
		return nil, nil
	}

	req := httptest.NewRequest(http.MethodGet, tailscaleServeStatusRoute, nil)
	rec := httptest.NewRecorder()
	sut.handleTailscaleServeStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response tailscaleServeStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !response.TailscaleInstalled {
		t.Fatalf("expected tailscale_installed=true")
	}
	if response.LoggedIn {
		t.Fatalf("expected logged_in=false")
	}
	if response.ServeStatus != "login_required" {
		t.Fatalf("expected serve_status=login_required, got %q", response.ServeStatus)
	}
	if response.SuggestedTarget != "http://127.0.0.1:3000" {
		t.Fatalf("expected TLS serve target, got %q", response.SuggestedTarget)
	}
	if response.SuggestedCommand != "" {
		t.Fatalf("expected no suggested command before login, got %q", response.SuggestedCommand)
	}
}

func TestHandleTailscaleServeStatus_LoggedInWithoutServe(t *testing.T) {
	sut := newTestAPIServer(t)

	originalLookPath := tailscaleLookPath
	originalRunner := tailscaleRunner
	originalFallbackPaths := tailscaleFallbackPaths
	t.Cleanup(func() {
		tailscaleLookPath = originalLookPath
		tailscaleRunner = originalRunner
		tailscaleFallbackPaths = originalFallbackPaths
	})

	tailscaleLookPath = func(file string) (string, error) {
		return "/usr/local/bin/tailscale", nil
	}
	tailscaleRunner = func(_ time.Duration, path string, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "status --json":
			return []byte(`{
				"BackendState": "Running",
				"CurrentTailnet": { "Name": "homelab.ts.net" },
				"Self": {
					"DNSName": "hub.homelab.ts.net.",
					"TailscaleIPs": ["100.101.102.103"]
				}
			}`), nil
		case "serve status --json":
			return nil, errors.New("serve json unsupported")
		case "serve status":
			return []byte("No serve config"), nil
		default:
			t.Fatalf("unexpected tailscale invocation: %v", args)
			return nil, nil
		}
	}

	req := httptest.NewRequest(http.MethodGet, tailscaleServeStatusRoute, nil)
	rec := httptest.NewRecorder()
	sut.handleTailscaleServeStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response tailscaleServeStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.ServeStatus != "off" {
		t.Fatalf("expected serve_status=off, got %q", response.ServeStatus)
	}
	if response.ServeConfigured {
		t.Fatalf("expected serve_configured=false")
	}
	if response.TSNetURL != "https://hub.homelab.ts.net" {
		t.Fatalf("expected ts.net URL, got %q", response.TSNetURL)
	}
	if response.SuggestedTarget != "http://127.0.0.1:3000" {
		t.Fatalf("expected console serve target, got %q", response.SuggestedTarget)
	}
	if response.SuggestedCommand != "tailscale serve --bg http://127.0.0.1:3000" {
		t.Fatalf("unexpected suggested command %q", response.SuggestedCommand)
	}
}

func TestHandleTailscaleServeStatus_Configured(t *testing.T) {
	t.Setenv(envTailscaleManaged, "true")

	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true

	originalLookPath := tailscaleLookPath
	originalRunner := tailscaleRunner
	originalFallbackPaths := tailscaleFallbackPaths
	t.Cleanup(func() {
		tailscaleLookPath = originalLookPath
		tailscaleRunner = originalRunner
		tailscaleFallbackPaths = originalFallbackPaths
	})

	tailscaleLookPath = func(file string) (string, error) {
		return "/usr/local/bin/tailscale", nil
	}
	tailscaleRunner = func(_ time.Duration, path string, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "status --json":
			return []byte(`{
				"BackendState": "Running",
				"CurrentTailnet": { "Name": "homelab.ts.net" },
				"Self": {
					"DNSName": "hub.homelab.ts.net.",
					"TailscaleIPs": ["100.101.102.103"]
				}
			}`), nil
		case "serve status --json":
			return []byte(`{
				"TCP": {
					"443": {
						"HTTPS": true,
						"Web": {
							"/": {
								"Proxy": "http://127.0.0.1:3000"
							}
						}
					}
				}
			}`), nil
		default:
			t.Fatalf("unexpected tailscale invocation: %v", args)
			return nil, nil
		}
	}

	req := httptest.NewRequest(http.MethodGet, tailscaleServeStatusRoute, nil)
	rec := httptest.NewRecorder()
	sut.handleTailscaleServeStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response tailscaleServeStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.ServeStatus != "configured" {
		t.Fatalf("expected serve_status=configured, got %q", response.ServeStatus)
	}
	if !response.ServeConfigured {
		t.Fatalf("expected serve_configured=true")
	}
	if response.ServeTarget != "http://127.0.0.1:3000" {
		t.Fatalf("expected detected serve target, got %q", response.ServeTarget)
	}
	if response.ManagementMode != "managed" {
		t.Fatalf("expected management_mode=managed, got %q", response.ManagementMode)
	}
	if !response.CanManage {
		t.Fatalf("expected can_manage=true")
	}
}

func TestHandleTailscaleServeStatus_FindsMacOSAppBundleBinary(t *testing.T) {
	sut := newTestAPIServer(t)

	originalLookPath := tailscaleLookPath
	originalRunner := tailscaleRunner
	originalFallbackPaths := tailscaleFallbackPaths
	t.Cleanup(func() {
		tailscaleLookPath = originalLookPath
		tailscaleRunner = originalRunner
		tailscaleFallbackPaths = originalFallbackPaths
	})

	appBinary := filepath.Join(t.TempDir(), "Tailscale.app", "Contents", "MacOS", "Tailscale")
	if err := os.MkdirAll(filepath.Dir(appBinary), 0o755); err != nil {
		t.Fatalf("mkdir app bundle path: %v", err)
	}
	if err := os.WriteFile(appBinary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write app bundle binary: %v", err)
	}

	tailscaleLookPath = func(file string) (string, error) {
		return "", errors.New("not on PATH")
	}
	tailscaleFallbackPaths = func() []string { return []string{appBinary} }
	tailscaleRunner = func(_ time.Duration, path string, args ...string) ([]byte, error) {
		if path != appBinary {
			t.Fatalf("expected app bundle binary path %q, got %q", appBinary, path)
		}
		switch strings.Join(args, " ") {
		case "status --json":
			return []byte(`{
				"BackendState": "Running",
				"CurrentTailnet": { "Name": "homelab.ts.net" },
				"Self": {
					"DNSName": "hub.homelab.ts.net.",
					"TailscaleIPs": ["100.101.102.103"]
				}
			}`), nil
		case "serve status --json":
			return []byte(`{}`), nil
		default:
			t.Fatalf("unexpected tailscale invocation: %v", args)
			return nil, nil
		}
	}

	req := httptest.NewRequest(http.MethodGet, tailscaleServeStatusRoute, nil)
	rec := httptest.NewRecorder()
	sut.handleTailscaleServeStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response tailscaleServeStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !response.TailscaleInstalled {
		t.Fatalf("expected tailscale_installed=true when app bundle binary exists")
	}
	if response.TSNetURL != "https://hub.homelab.ts.net" {
		t.Fatalf("expected ts.net URL, got %q", response.TSNetURL)
	}
}

func TestHandleTailscaleServeStatus_RespectsSavedModeChoice(t *testing.T) {
	sut := newTestAPIServer(t)
	if _, err := sut.runtimeStore.SaveRuntimeSettingOverrides(map[string]string{
		"remote_access.mode": "off",
	}); err != nil {
		t.Fatalf("failed to seed runtime overrides: %v", err)
	}

	originalLookPath := tailscaleLookPath
	originalRunner := tailscaleRunner
	originalFallbackPaths := tailscaleFallbackPaths
	t.Cleanup(func() {
		tailscaleLookPath = originalLookPath
		tailscaleRunner = originalRunner
		tailscaleFallbackPaths = originalFallbackPaths
	})

	tailscaleLookPath = func(file string) (string, error) {
		return "", errors.New("not found")
	}
	tailscaleFallbackPaths = func() []string { return nil }

	req := httptest.NewRequest(http.MethodGet, tailscaleServeStatusRoute, nil)
	rec := httptest.NewRecorder()
	sut.handleTailscaleServeStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response tailscaleServeStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.DesiredMode != "off" {
		t.Fatalf("expected desired_mode=off, got %q", response.DesiredMode)
	}
	if response.RecommendationState != "disabled_by_choice" {
		t.Fatalf("expected disabled_by_choice recommendation state, got %q", response.RecommendationState)
	}
}

func TestHandleTailscaleServeStatus_MethodNotAllowed(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodPut, tailscaleServeStatusRoute, nil)
	rec := httptest.NewRecorder()
	sut.handleTailscaleServeStatus(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleTailscaleServeStatusMutation_RequiresAdmin(t *testing.T) {
	t.Setenv(envTailscaleManaged, "true")
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodPost, tailscaleServeStatusRoute, strings.NewReader(`{"action":"apply"}`))
	req = req.WithContext(contextWithPrincipal(req.Context(), "operator-1", "operator"))
	rec := httptest.NewRecorder()
	sut.handleTailscaleServeStatus(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleTailscaleServeStatusMutation_AppliesManagedServe(t *testing.T) {
	t.Setenv(envTailscaleManaged, "true")
	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true

	originalLookPath := tailscaleLookPath
	originalRunner := tailscaleRunner
	t.Cleanup(func() {
		tailscaleLookPath = originalLookPath
		tailscaleRunner = originalRunner
	})

	applied := false
	tailscaleLookPath = func(file string) (string, error) {
		return "/usr/local/bin/tailscale", nil
	}
	tailscaleRunner = func(_ time.Duration, path string, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "status --json":
			return []byte(`{"BackendState":"Running","CurrentTailnet":{"Name":"homelab.ts.net"},"Self":{"DNSName":"hub.homelab.ts.net."}}`), nil
		case "serve status --json":
			if applied {
				return []byte(`{"TCP":{"443":{"HTTPS":true,"Web":{"/":{"Proxy":"http://127.0.0.1:3000"}}}}}`), nil
			}
			return []byte(`{}`), nil
		case "serve --bg --yes http://127.0.0.1:3000":
			applied = true
			return []byte("Serving"), nil
		default:
			t.Fatalf("unexpected tailscale invocation: %v", args)
			return nil, nil
		}
	}

	req := httptest.NewRequest(http.MethodPost, tailscaleServeStatusRoute, strings.NewReader(`{"action":"apply"}`))
	req = req.WithContext(contextWithPrincipal(req.Context(), "owner", "owner"))
	rec := httptest.NewRecorder()
	sut.handleTailscaleServeStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response tailscaleServeStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !response.ServeConfigured {
		t.Fatalf("expected serve to be configured after apply")
	}
	if response.SuggestedCommand != "tailscale serve --bg http://127.0.0.1:3000" {
		t.Fatalf("unexpected suggested command %q", response.SuggestedCommand)
	}
}
