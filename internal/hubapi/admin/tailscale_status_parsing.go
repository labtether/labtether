package admin

import (
	"encoding/json"
	"strings"
)

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
