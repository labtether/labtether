package tailscale

import (
	"errors"
	"testing"
	"time"
)

func TestReadMetadataWithMissingBinary(t *testing.T) {
	metadata := ReadMetadataWith(
		func(string) (string, error) { return "", errors.New("not found") },
		func(time.Duration, string, ...string) ([]byte, error) {
			t.Fatalf("runner should not be called when tailscale is missing")
			return nil, nil
		},
	)

	if got := metadata["tailscale_installed"]; got != "false" {
		t.Fatalf("tailscale_installed=%q, want false", got)
	}
}

func TestReadMetadataWithStatusAndVersionJSON(t *testing.T) {
	metadata := ReadMetadataWith(
		func(name string) (string, error) { return "/usr/local/bin/" + name, nil },
		func(_ time.Duration, _ string, args ...string) ([]byte, error) {
			if len(args) >= 2 && args[0] == "status" && args[1] == "--json" {
				return []byte(`{
					"BackendState":"Running",
					"CurrentTailnet":{"Name":"labnet","MagicDNSSuffix":"labnet.ts.net."},
					"Self":{"DNSName":"mac-mini.labnet.ts.net","TailscaleIPs":["100.64.0.9","fd7a:115c:a1e0::9"],"ExitNode":false}
				}`), nil
			}
			if len(args) >= 2 && args[0] == "version" && args[1] == "--json" {
				return []byte(`{"short":"1.66.4"}`), nil
			}
			return nil, errors.New("unexpected command")
		},
	)

	if got := metadata["tailscale_installed"]; got != "true" {
		t.Fatalf("tailscale_installed=%q, want true", got)
	}
	if got := metadata["tailscale_backend_state"]; got != "Running" {
		t.Fatalf("tailscale_backend_state=%q, want Running", got)
	}
	if got := metadata["tailscale_tailnet"]; got != "labnet" {
		t.Fatalf("tailscale_tailnet=%q, want labnet", got)
	}
	if got := metadata["tailscale_self_dns_name"]; got != "mac-mini.labnet.ts.net" {
		t.Fatalf("tailscale_self_dns_name=%q, want mac-mini.labnet.ts.net", got)
	}
	if got := metadata["tailscale_self_tailscale_ip"]; got != "100.64.0.9,fd7a:115c:a1e0::9" {
		t.Fatalf("tailscale_self_tailscale_ip=%q, want both IPs", got)
	}
	if got := metadata["tailscale_exit_node"]; got != "false" {
		t.Fatalf("tailscale_exit_node=%q, want false", got)
	}
	if got := metadata["tailscale_version"]; got != "1.66.4" {
		t.Fatalf("tailscale_version=%q, want 1.66.4", got)
	}
}

func TestReadMetadataWithVersionFallbackAndExitNodeStatus(t *testing.T) {
	metadata := ReadMetadataWith(
		func(name string) (string, error) { return "/usr/local/bin/" + name, nil },
		func(_ time.Duration, _ string, args ...string) ([]byte, error) {
			if len(args) >= 2 && args[0] == "status" && args[1] == "--json" {
				return []byte(`{
					"BackendState":"Running",
					"CurrentTailnet":{"MagicDNSSuffix":"lab.example.ts.net."},
					"ExitNodeStatus":{"ID":"n123"},
					"Self":{"DNSName":"node.lab.example.ts.net","TailscaleIPs":["100.100.10.10"]}
				}`), nil
			}
			if len(args) >= 2 && args[0] == "version" && args[1] == "--json" {
				return nil, errors.New("json not supported")
			}
			if len(args) == 1 && args[0] == "version" {
				return []byte("1.70.0\n"), nil
			}
			return nil, errors.New("unexpected command")
		},
	)

	if got := metadata["tailscale_tailnet"]; got != "lab.example.ts.net" {
		t.Fatalf("tailscale_tailnet=%q, want lab.example.ts.net", got)
	}
	if got := metadata["tailscale_exit_node"]; got != "true" {
		t.Fatalf("tailscale_exit_node=%q, want true", got)
	}
	if got := metadata["tailscale_version"]; got != "1.70.0" {
		t.Fatalf("tailscale_version=%q, want 1.70.0", got)
	}
}

func TestParseTailscaleVersionRejectsNonVersionOutput(t *testing.T) {
	if got := parseTailscaleVersion([]byte("The operation couldn't be completed.")); got != "" {
		t.Fatalf("parseTailscaleVersion returned %q for non-version output", got)
	}
}

func TestParseTailscaleVersionExtractsFromVerboseOutput(t *testing.T) {
	got := parseTailscaleVersion([]byte("tailscale v1.72.0-tabcdef go1.22.5"))
	if got != "1.72.0-tabcdef" {
		t.Fatalf("parseTailscaleVersion=%q, want 1.72.0-tabcdef", got)
	}
}
