package resources

import (
	"context"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/fileproto"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
	"net/http"
	"strings"
	"time"
)

// --- Test (stateless) ---

type fileConnectionTestRequest struct {
	Protocol    string         `json:"protocol"`
	Host        string         `json:"host"`
	Port        *int           `json:"port,omitempty"`
	InitialPath string         `json:"initial_path"`
	Username    string         `json:"username"`
	Secret      string         `json:"secret"` // #nosec G117 -- Request payload intentionally carries runtime credential material.
	Passphrase  string         `json:"passphrase,omitempty"`
	AuthMethod  string         `json:"auth_method"`
	ExtraConfig map[string]any `json:"extra_config,omitempty"`
}

func (d *Deps) handleFileConnectionTestStateless(w http.ResponseWriter, r *http.Request) {
	var req fileConnectionTestRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		return
	}
	req.Protocol = strings.TrimSpace(req.Protocol)
	req.Host = strings.TrimSpace(req.Host)
	req.Username = strings.TrimSpace(req.Username)
	req.AuthMethod = strings.TrimSpace(req.AuthMethod)
	req.InitialPath = strings.TrimSpace(req.InitialPath)

	if req.Protocol == "" || req.Host == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "protocol and host are required")
		return
	}

	port := 0
	if req.Port != nil {
		port = *req.Port
	}
	if port == 0 {
		port = fileproto.DefaultPort(req.Protocol)
	}

	initialPath := req.InitialPath
	if initialPath == "" {
		initialPath = "/"
	}

	config := fileproto.ConnectionConfig{
		Protocol:    req.Protocol,
		Host:        req.Host,
		Port:        port,
		Username:    req.Username,
		Secret:      strings.TrimSpace(req.Secret),
		Passphrase:  strings.TrimSpace(req.Passphrase),
		AuthMethod:  req.AuthMethod,
		InitialPath: initialPath,
		ExtraConfig: req.ExtraConfig,
	}
	normalizedExtra, err := NormalizeFileConnectionExtraConfig(config.Protocol, config.ExtraConfig)
	if err != nil {
		servicehttp.WriteJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": err.Error()})
		return
	}
	config.ExtraConfig = normalizedExtra
	if config.Protocol == "sftp" {
		if _, pinned := normalizedExtra["host_key"]; !pinned {
			start := time.Now()
			hostKey, fingerprint, discoverErr := fileproto.DiscoverSFTPHostKey(r.Context(), config.Host, config.Port)
			if discoverErr != nil {
				securityruntime.Logf("file protocol: SFTP host key discovery failed: %v", discoverErr)
				servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
					"success": false,
					"error":   "failed to read the SFTP server key",
				})
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
				"success":                        false,
				"requires_host_key_confirmation": true,
				"error":                          "review the SFTP server key, then test again to confirm it",
				"host_key":                       hostKey,
				"fingerprint":                    fingerprint,
				"latency_ms":                     time.Since(start).Milliseconds(),
			})
			return
		}
	}

	result := d.testFileConnection(r.Context(), config)
	servicehttp.WriteJSON(w, http.StatusOK, result)
}

// --- Test (saved) ---

func (d *Deps) handleFileConnectionTestSaved(w http.ResponseWriter, r *http.Request, connID string) {
	if d.SecretsManager == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential encryption not configured")
		return
	}
	if d.CredentialStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential store unavailable")
		return
	}

	fc, err := d.FileConnectionStore.GetFileConnection(r.Context(), connID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "file connection not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load file connection")
		return
	}
	if !d.canAccessFileConnection(r, fc) {
		servicehttp.WriteError(w, http.StatusForbidden, "file connection access denied")
		return
	}

	config, err := d.buildConnectionConfig(fc)
	if err != nil {
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	result := d.testFileConnection(r.Context(), config)
	servicehttp.WriteJSON(w, http.StatusOK, result)
}

func (d *Deps) testFileConnection(ctx context.Context, config fileproto.ConnectionConfig) map[string]any {
	if d.FileProtoPool == nil {
		return map[string]any{"success": false, "error": "file protocol pool not initialized"}
	}

	// Use a temporary connection ID for testing so we don't pollute the pool.
	testID := "test-" + GenerateRequestID()
	start := time.Now()

	fs, err := d.FileProtoPool.Get(ctx, testID, config)
	if err != nil {
		return map[string]any{
			"success": false,
			"error":   fmt.Sprintf("connection failed: %s", err.Error()),
		}
	}
	defer d.FileProtoPool.Remove(testID)

	// Try listing the initial path to verify the connection works end-to-end.
	_, err = fs.List(ctx, config.InitialPath, false)
	latencyMs := time.Since(start).Milliseconds()
	if err != nil {
		return map[string]any{
			"success":    false,
			"error":      fmt.Sprintf("listing failed: %s", err.Error()),
			"latency_ms": latencyMs,
		}
	}

	result := map[string]any{
		"success":    true,
		"latency_ms": latencyMs,
	}

	// For SFTP connections, surface the captured host key fingerprint for TOFU.
	// The frontend can display this to the user and store it in extra_config.
	if sftpClient, ok := fs.(*fileproto.SFTPClient); ok {
		if sftpClient.CapturedHostKey != "" {
			result["host_key"] = sftpClient.CapturedHostKey
			result["fingerprint"] = sftpClient.CapturedFingerprint
		}
	}

	return result
}
