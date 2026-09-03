package resources

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/securityruntime"
)

const (
	maxFileConnectionHostKeyLen = 16 * 1024
	maxFileConnectionFieldLen   = 255
	maxWebDAVBasePathLen        = 2048
)

// NormalizeFileConnectionExtraConfig defines the complete non-secret schema
// for protocol-specific connection options. Credentials must use the encrypted
// credential profile, never this API-visible JSON object.
func NormalizeFileConnectionExtraConfig(protocol string, config map[string]any) (map[string]any, error) {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	normalized := map[string]any{}
	for key, value := range config {
		switch protocol + ":" + key {
		case "sftp:host_key":
			hostKey, ok := value.(string)
			hostKey = strings.TrimSpace(hostKey)
			if !ok || hostKey == "" || len(hostKey) > maxFileConnectionHostKeyLen || containsControlRune(hostKey) {
				return nil, fmt.Errorf("host_key must be a valid public host key up to %d bytes", maxFileConnectionHostKeyLen)
			}
			if _, _, _, rest, err := ssh.ParseAuthorizedKey([]byte(hostKey)); err != nil || len(strings.TrimSpace(string(rest))) != 0 {
				return nil, errors.New("host_key must contain exactly one valid SSH public key")
			}
			normalized[key] = hostKey
		case "ftp:ftp_tls", "ftp:ftp_passive", "ftp:ftp_allow_cleartext", "webdav:webdav_tls":
			flag, ok := value.(bool)
			if !ok {
				return nil, fmt.Errorf("%s must be a boolean", key)
			}
			normalized[key] = flag
		case "webdav:webdav_tls_skip_verify":
			flag, ok := value.(bool)
			if !ok {
				return nil, fmt.Errorf("%s must be a boolean", key)
			}
			if flag && !securityruntime.InsecureTransportAllowed() {
				return nil, errors.New("webdav_tls_skip_verify requires LABTETHER_ALLOW_INSECURE_TRANSPORT=true")
			}
			normalized[key] = flag
		case "smb:smb_share", "smb:smb_domain":
			text, ok := value.(string)
			text = strings.TrimSpace(text)
			if !ok || text == "" || len(text) > maxFileConnectionFieldLen || containsControlRune(text) {
				return nil, fmt.Errorf("%s must be 1-%d bytes without control characters", key, maxFileConnectionFieldLen)
			}
			if key == "smb_share" && strings.ContainsAny(text, "/\\") {
				return nil, errors.New("smb_share must be a share name, not a path")
			}
			normalized[key] = text
		case "webdav:webdav_base_path":
			basePath, ok := value.(string)
			basePath = strings.TrimSpace(basePath)
			if !ok || len(basePath) > maxWebDAVBasePathLen || containsControlRune(basePath) || strings.ContainsAny(basePath, "?#") || strings.Contains(basePath, "://") {
				return nil, fmt.Errorf("webdav_base_path must be a URL path up to %d bytes", maxWebDAVBasePathLen)
			}
			if basePath != "" && !strings.HasPrefix(basePath, "/") {
				basePath = "/" + basePath
			}
			normalized[key] = basePath
		default:
			return nil, fmt.Errorf("unsupported %s extra_config key: %s", protocol, key)
		}
	}
	if protocol == "ftp" {
		useTLS := true
		if configured, ok := normalized["ftp_tls"].(bool); ok {
			useTLS = configured
		} else {
			normalized["ftp_tls"] = true
		}
		allowCleartext, _ := normalized["ftp_allow_cleartext"].(bool)
		if useTLS && allowCleartext {
			return nil, errors.New("ftp_allow_cleartext cannot be enabled when ftp_tls is true")
		}
		if !useTLS && (!allowCleartext || !securityruntime.InsecureTransportAllowed()) {
			return nil, errors.New("plain FTP requires ftp_allow_cleartext=true and LABTETHER_ALLOW_INSECURE_TRANSPORT=true")
		}
	}
	return normalized, nil
}

func containsControlRune(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func sanitizeLegacyFileConnectionExtraConfig(protocol string, config map[string]any) map[string]any {
	if normalized, err := NormalizeFileConnectionExtraConfig(protocol, config); err == nil {
		return normalized
	}
	if strings.EqualFold(strings.TrimSpace(protocol), "ftp") {
		known := map[string]any{}
		for _, key := range []string{"ftp_tls", "ftp_passive", "ftp_allow_cleartext"} {
			if value, ok := config[key]; ok {
				known[key] = value
			}
		}
		if normalized, err := NormalizeFileConnectionExtraConfig(protocol, known); err == nil {
			return normalized
		}
	}
	out := map[string]any{}
	for key, value := range config {
		if normalized, err := NormalizeFileConnectionExtraConfig(protocol, map[string]any{key: value}); err == nil {
			out[key] = normalized[key]
		}
	}
	if strings.EqualFold(strings.TrimSpace(protocol), "ftp") {
		out["ftp_tls"] = true
	}
	return out
}

// preserveExistingSFTPHostKey prevents an ordinary partial update from
// silently clearing a trusted server identity. Replacing the pin requires an
// explicit new host_key value.
func preserveExistingSFTPHostKey(protocol string, current, requested map[string]any) map[string]any {
	if strings.ToLower(strings.TrimSpace(protocol)) != "sftp" {
		return requested
	}
	if _, explicitlySet := requested["host_key"]; explicitlySet {
		return requested
	}
	existing, pinned := current["host_key"]
	if !pinned {
		return requested
	}
	merged := make(map[string]any, len(requested)+1)
	for key, value := range requested {
		merged[key] = value
	}
	merged["host_key"] = existing
	return merged
}
