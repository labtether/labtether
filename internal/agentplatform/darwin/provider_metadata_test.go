package darwin

import (
	"errors"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentplatform/tailscale"
)

func TestReadTailscaleMetadataWithMissingBinary(t *testing.T) {
	metadata := tailscale.ReadMetadataWith(
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

func TestReadTailscaleMetadataWithStatusAndVersionJSON(t *testing.T) {
	metadata := tailscale.ReadMetadataWith(
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

func TestReadTailscaleMetadataWithVersionFallbackAndExitNodeStatus(t *testing.T) {
	metadata := tailscale.ReadMetadataWith(
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

func TestReadCapabilityMetadataWithToolingPresent(t *testing.T) {
	metadata := readCapabilityMetadataWith(func(name string) (string, error) {
		switch name {
		case "launchctl", "brew", "log", "networksetup":
			return "/usr/bin/" + name, nil
		default:
			return "", errors.New("not found")
		}
	})

	if got := metadata["cap_services"]; got != "list,action" {
		t.Fatalf("cap_services=%q, want list,action", got)
	}
	if got := metadata["cap_packages"]; got != "list,action" {
		t.Fatalf("cap_packages=%q, want list,action", got)
	}
	if got := metadata["cap_logs"]; got != "stored,query,stream" {
		t.Fatalf("cap_logs=%q, want stored,query,stream", got)
	}
	if got := metadata["cap_network"]; got != "list,action" {
		t.Fatalf("cap_network=%q, want list,action", got)
	}
	if got := metadata["service_backend"]; got != "launchd" {
		t.Fatalf("service_backend=%q, want launchd", got)
	}
	if got := metadata["package_backend"]; got != "brew" {
		t.Fatalf("package_backend=%q, want brew", got)
	}
	if got := metadata["log_backend"]; got != "oslog" {
		t.Fatalf("log_backend=%q, want oslog", got)
	}
	if got := metadata["network_action_backend"]; got != "networksetup" {
		t.Fatalf("network_action_backend=%q, want networksetup", got)
	}
}

func TestReadCapabilityMetadataWithNoTooling(t *testing.T) {
	metadata := readCapabilityMetadataWith(func(string) (string, error) {
		return "", errors.New("not found")
	})

	if got := metadata["cap_services"]; got != "" {
		t.Fatalf("cap_services=%q, want empty", got)
	}
	if got := metadata["cap_packages"]; got != "" {
		t.Fatalf("cap_packages=%q, want empty", got)
	}
	if got := metadata["cap_logs"]; got != "stored" {
		t.Fatalf("cap_logs=%q, want stored", got)
	}
	if got := metadata["cap_network"]; got != "list" {
		t.Fatalf("cap_network=%q, want list", got)
	}
}
