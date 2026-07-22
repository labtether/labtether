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
		case "ftp:ftp_tls", "ftp:ftp_passive", "webdav:webdav_tls":
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
	out := map[string]any{}
	for key, value := range config {
		if normalized, err := NormalizeFileConnectionExtraConfig(protocol, map[string]any{key: value}); err == nil {
			out[key] = normalized[key]
		}
	}
	return out
}
