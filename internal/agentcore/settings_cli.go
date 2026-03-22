package agentcore

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	dockerpkg "github.com/labtether/labtether/internal/agentcore/docker"
)

var (
	settingsCLIDockerPing = func(_ context.Context, endpoint string) error {
		if dockerpkg.PingDockerEndpoint(endpoint, 5*time.Second) {
			return nil
		}
		return fmt.Errorf("docker endpoint unreachable: %s", endpoint)
	}
	settingsCLIEnsureDeviceIdentity = ensureDeviceIdentity
	settingsCLISelfUpdate           = checkAndApplySelfUpdateWithOptions
)

// HandleCLICommand executes agent-local CLI commands.
// Returns handled=true when args represent a recognized CLI command.
func HandleCLICommand(cfg RuntimeConfig, args []string) (handled bool, exitCode int) {
	if len(args) == 0 {
		return false, 0
	}

	switch strings.TrimSpace(strings.ToLower(args[0])) {
	case "settings":
		return true, runSettingsCommand(cfg, args[1:])
	case "identity":
		return true, runIdentityCommand(cfg, args[1:])
	case "update":
		return true, runUpdateCommand(cfg, args[1:])
	case "help", "-h", "--help":
		if len(args) > 1 {
			return true, printSubcommandHelp(cfg, args[1])
		}
		printTopLevelHelp(cfg)
		return true, 0
	default:
		fmt.Printf("unknown command: %s\n", args[0])
		printTopLevelHelp(cfg)
		return true, 1
	}
}

func printTopLevelHelp(cfg RuntimeConfig) {
	version := strings.TrimSpace(cfg.Version)
	if version == "" {
		version = "dev"
	}
	fmt.Printf("labtether-agent %s\n", version)
	fmt.Println()
	fmt.Println("LabTether endpoint agent for telemetry, remote access, and automation.")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  labtether-agent                         Start the agent daemon")
	fmt.Println("  labtether-agent <command> [options]      Run a CLI command")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  settings    Manage agent settings (show, set, wizard, test)")
	fmt.Println("  identity    Show device identity and key fingerprint")
	fmt.Println("  update      Self-update the agent binary")
	fmt.Println("  help        Show this help message")
	fmt.Println()
	fmt.Println("Run 'labtether-agent help <command>' for detailed usage of a command.")
}

func printSubcommandHelp(cfg RuntimeConfig, command string) int {
	switch strings.TrimSpace(strings.ToLower(command)) {
	case "settings":
		printSettingsHelp(cfg)
		return 0
	case "identity":
		printIdentityHelp(cfg)
		return 0
	case "update":
		printUpdateHelp()
		return 0
	default:
		fmt.Printf("unknown command: %s\n", command)
		fmt.Println()
		fmt.Println("Available commands: settings, identity, update")
		return 1
	}
}

func printSettingsHelp(cfg RuntimeConfig) {
	fmt.Println("Manage agent settings.")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  labtether-agent settings show                        Show current settings as JSON")
	fmt.Println("  labtether-agent settings set <key> <value>           Set a single setting")
	fmt.Println("  labtether-agent settings wizard                      Interactive setup wizard")
	fmt.Println("  labtether-agent settings test docker [endpoint]      Test Docker endpoint connectivity")
	fmt.Println()
	fmt.Println("Settings file:")
	settingsPath := strings.TrimSpace(cfg.AgentSettingsPath)
	if settingsPath == "" {
		settingsPath = defaultSettingsFile
	}
	fmt.Printf("  %s\n", settingsPath)
	fmt.Println()
	fmt.Println("Available setting keys:")
	for _, def := range AgentSettingDefinitions() {
		line := fmt.Sprintf("  %-50s %s", def.Key, def.Description)
		if def.DefaultValue != "" {
			line += fmt.Sprintf(" (default: %s)", def.DefaultValue)
		}
		fmt.Println(line)
	}
	fmt.Println()
	fmt.Println("Environment variables:")
	fmt.Println("  LABTETHER_API_BASE_URL                     Hub API base URL")
	fmt.Println("  LABTETHER_WS_URL                           Hub WebSocket URL")
	fmt.Println("  LABTETHER_API_TOKEN                        Agent authentication token")
	fmt.Println("  LABTETHER_ENROLLMENT_TOKEN                 Auto-enrollment token")
	fmt.Println("  LABTETHER_ENROLLMENT_TOKEN_FILE            File-based enrollment token source")
	fmt.Printf("  LABTETHER_TOKEN_FILE                       Persisted token file path (default: %s)\n", defaultTokenFile)
	fmt.Printf("  LABTETHER_AGENT_SETTINGS_FILE              Settings file path (default: %s)\n", defaultSettingsFile)
	fmt.Println("  LABTETHER_DOCKER_SOCKET                    Docker socket path (default: /var/run/docker.sock)")
	fmt.Println("  LABTETHER_DOCKER_ENABLED                   Docker collector mode: auto|true|false (default: auto)")
	fmt.Println("  LABTETHER_DOCKER_DISCOVERY_INTERVAL        Docker discovery interval (default: 30s)")
	fmt.Println("  LABTETHER_FILES_ROOT_MODE                  File browser scope: home|full (default: home)")
	fmt.Println("  LABTETHER_WEBRTC_ENABLED                   Enable WebRTC streaming (default: true)")
	fmt.Println("  LABTETHER_WEBRTC_STUN_URL                  STUN server URL")
	fmt.Println("  LABTETHER_WEBRTC_TURN_URL                  TURN server URL")
	fmt.Println("  LABTETHER_WEBRTC_TURN_USER                 TURN username")
	fmt.Println("  LABTETHER_WEBRTC_TURN_PASS                 TURN password")
	fmt.Println("  LABTETHER_WEBRTC_TURN_PASS_FILE            File-based TURN password source")
	fmt.Println("  LABTETHER_CAPTURE_FPS                      WebRTC capture FPS (default: 30)")
	fmt.Println("  LABTETHER_ALLOW_REMOTE_OVERRIDES           Allow hub-side settings updates (default: true)")
	fmt.Println("  LABTETHER_LOG_LEVEL                        Log verbosity: debug|info|warn|error (default: info)")
	fmt.Println("  LABTETHER_LOW_POWER_MODE                   Reduce CPU/memory with longer intervals (default: false)")
	fmt.Println("  LABTETHER_LOG_STREAM_ENABLED               Enable system log streaming (default: true)")
	fmt.Println("  LABTETHER_TLS_CA_FILE                      Custom CA cert for hub TLS verification")
	fmt.Println("  LABTETHER_TLS_SKIP_VERIFY                  Skip TLS cert verification (default: false)")
	fmt.Println("  LABTETHER_ALLOW_INSECURE_TRANSPORT         Allow HTTP/WS instead of HTTPS/WSS (default: false)")
	fmt.Println("  LABTETHER_AUTO_UPDATE                      Enable auto-update on startup (default: true)")
	fmt.Println("  LABTETHER_AUTO_UPDATE_CHECK_URL             Custom update check endpoint")
	fmt.Println("  AGENT_ASSET_ID                             Override asset ID (default: hostname)")
	fmt.Println("  AGENT_GROUP_ID                             Asset group ID")
	fmt.Println("  AGENT_NAME                                 Agent name (default: labtether-agent)")
	fmt.Println("  AGENT_PORT                                 Local HTTP API port (default: 8090)")
	fmt.Println("  AGENT_COLLECT_INTERVAL                     Telemetry collection interval (default: 10s)")
	fmt.Println("  AGENT_HEARTBEAT_INTERVAL                   Heartbeat publish interval (default: 20s)")
	fmt.Println("  LABTETHER_SERVICES_DISCOVERY_INTERVAL      Web service discovery interval (default: 60s)")
	fmt.Println("  LABTETHER_SERVICES_DISCOVERY_DOCKER_ENABLED          Docker service discovery (default: true)")
	fmt.Println("  LABTETHER_SERVICES_DISCOVERY_PROXY_ENABLED           Reverse-proxy API discovery (default: true)")
	fmt.Println("  LABTETHER_SERVICES_DISCOVERY_PORT_SCAN_ENABLED       Local port scan discovery (default: true)")
	fmt.Println("  LABTETHER_SERVICES_DISCOVERY_LAN_SCAN_ENABLED        LAN CIDR scan discovery (default: false)")
	fmt.Println("  LABTETHER_SERVICES_DISCOVERY_LAN_SCAN_CIDRS          LAN CIDR ranges to scan")
	fmt.Println("  LABTETHER_SERVICES_DISCOVERY_LAN_SCAN_PORTS          LAN scan port list")
	fmt.Println("  LABTETHER_SERVICES_DISCOVERY_LAN_SCAN_MAX_HOSTS      Max LAN hosts per scan (default: 64)")
}

func printIdentityHelp(cfg RuntimeConfig) {
	fmt.Println("Show device identity and key fingerprint.")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  labtether-agent identity              Show device identity (default)")
	fmt.Println("  labtether-agent identity show         Show device identity")
	fmt.Println()
	fmt.Println("Output includes hostname, device fingerprint, key algorithm, and key file paths.")
	fmt.Println()
	fmt.Println("Key files:")
	keyPath := strings.TrimSpace(cfg.DeviceKeyPath)
	if keyPath == "" {
		keyPath = defaultDeviceKeyFile
	}
	pubKeyPath := strings.TrimSpace(cfg.DevicePublicKeyPath)
	if pubKeyPath == "" {
		pubKeyPath = defaultDevicePublicKeyFile
	}
	fpPath := strings.TrimSpace(cfg.DeviceFingerprintPath)
	if fpPath == "" {
		fpPath = defaultDeviceFingerprintFile
	}
	fmt.Printf("  Private key:   %s\n", keyPath)
	fmt.Printf("  Public key:    %s\n", pubKeyPath)
	fmt.Printf("  Fingerprint:   %s\n", fpPath)
	fmt.Println()
	fmt.Println("Environment variables:")
	fmt.Println("  LABTETHER_DEVICE_KEY_FILE              Device private key path")
	fmt.Println("  LABTETHER_DEVICE_PUBLIC_KEY_FILE        Device public key path")
	fmt.Println("  LABTETHER_DEVICE_FINGERPRINT_FILE       Device fingerprint file path")
}

func printUpdateHelp() {
	fmt.Println("Self-update the agent binary.")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  labtether-agent update self             Check for and apply updates")
	fmt.Println("  labtether-agent update self --force     Force re-download even if up to date")
	fmt.Println()
	fmt.Println("The update command downloads the latest agent binary from the hub or a")
	fmt.Println("configured update endpoint, verifies its checksum, and replaces the")
	fmt.Println("running binary. Restart the labtether-agent service after updating.")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --force, -f    Force update even if the current binary matches")
	fmt.Println()
	fmt.Println("Environment variables:")
	fmt.Println("  LABTETHER_AUTO_UPDATE                             Enable auto-update on startup (default: true)")
	fmt.Println("  LABTETHER_AUTO_UPDATE_CHECK_URL                   Custom update metadata endpoint")
	fmt.Println("  LABTETHER_AUTO_UPDATE_TRUSTED_PUBLIC_KEY          Ed25519 public key for signature verification")
	fmt.Println("  LABTETHER_AUTO_UPDATE_ALLOW_EXTERNAL_DOWNLOAD     Allow download from non-origin URLs (default: false)")
}

func runSettingsCommand(cfg RuntimeConfig, args []string) int {
	if len(args) == 0 {
		printSettingsHelp(cfg)
		return 0
	}
	switch strings.TrimSpace(strings.ToLower(args[0])) {
	case "help", "-h", "--help":
		printSettingsHelp(cfg)
		return 0
	case "show":
		values := AgentSettingValuesFromConfigSnapshot(configSnapshotFromRuntimeConfig(cfg))
		payload, _ := json.MarshalIndent(values, "", "  ")
		fmt.Println(string(payload))
		return 0
	case "set":
		if len(args) < 3 {
			fmt.Println("usage: labtether-agent settings set <key> <value>")
			return 1
		}
		key := strings.TrimSpace(args[1])
		value := strings.TrimSpace(strings.Join(args[2:], " "))
		definition, ok := AgentSettingDefinitionByKey(key)
		if !ok {
			fmt.Printf("unknown setting key: %s\n", key)
			return 1
		}
		key = definition.Key
		normalized, err := NormalizeAgentSettingValue(key, value)
		if err != nil {
			fmt.Printf("invalid value: %v\n", err)
			return 1
		}
		current, err := LoadAgentSettingsFile(cfg.AgentSettingsPath)
		if err != nil {
			fmt.Printf("failed to load settings file: %v\n", err)
			return 1
		}
		current[key] = normalized
		if err := SaveAgentSettingsFile(cfg.AgentSettingsPath, current); err != nil {
			fmt.Printf("failed to save settings file: %v\n", err)
			return 1
		}
		fmt.Printf("saved %s=%s\n", key, normalized)
		fmt.Println("restart labtether-agent service for restart-required settings.")
		return 0
	case "wizard":
		return runSettingsWizard(cfg)
	case "test":
		if len(args) < 2 {
			fmt.Println("usage: labtether-agent settings test docker [endpoint]")
			return 1
		}
		sub := strings.TrimSpace(strings.ToLower(args[1]))
		if sub != "docker" {
			fmt.Printf("unknown test target: %s\n", sub)
			return 1
		}
		endpoint := strings.TrimSpace(cfg.DockerSocket)
		if len(args) >= 3 {
			endpoint = strings.TrimSpace(strings.Join(args[2:], " "))
			normalized, err := NormalizeAgentSettingValue(SettingKeyDockerEndpoint, endpoint)
			if err != nil {
				fmt.Printf("invalid docker endpoint: %v\n", err)
				return 1
			}
			endpoint = normalized
		}
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		if err := settingsCLIDockerPing(ctx, endpoint); err != nil {
			fmt.Printf("docker endpoint check failed: %v\n", err)
			return 1
		}
		fmt.Printf("docker endpoint reachable: %s\n", endpoint)
		return 0
	default:
		fmt.Printf("unknown settings command: %s\n", args[0])
		return 1
	}
}

func runSettingsWizard(cfg RuntimeConfig) int {
	reader := bufio.NewReader(os.Stdin)
	fileValues, err := LoadAgentSettingsFile(cfg.AgentSettingsPath)
	if err != nil {
		fmt.Printf("failed to load settings file: %v\n", err)
		return 1
	}
	current := AgentSettingValuesFromConfigSnapshot(configSnapshotFromRuntimeConfig(cfg))
	for key, value := range fileValues {
		current[key] = value
	}

	mode := promptWithDefault(reader, "Docker mode (auto|true|false)", current[SettingKeyDockerEnabled])
	endpoint := current[SettingKeyDockerEndpoint]
	if strings.EqualFold(strings.TrimSpace(mode), "true") || strings.EqualFold(strings.TrimSpace(mode), "auto") {
		endpoint = promptWithDefault(reader, "Docker endpoint (/var/run/docker.sock, unix://..., https://...)", endpoint)
	}
	filesRootMode := promptWithDefault(reader, "File access scope (home|full)", current[SettingKeyFilesRootMode])
	allowRemote := promptWithDefault(reader, "Allow remote hub overrides (true|false)", current[SettingKeyAllowRemoteOverrides])

	updates := map[string]string{
		SettingKeyDockerEnabled:        mode,
		SettingKeyDockerEndpoint:       endpoint,
		SettingKeyFilesRootMode:        filesRootMode,
		SettingKeyAllowRemoteOverrides: allowRemote,
	}

	for key, value := range updates {
		normalized, err := NormalizeAgentSettingValue(key, value)
		if err != nil {
			fmt.Printf("invalid %s value: %v\n", key, err)
			return 1
		}
		fileValues[key] = normalized
	}

	if err := SaveAgentSettingsFile(cfg.AgentSettingsPath, fileValues); err != nil {
		fmt.Printf("failed to save settings: %v\n", err)
		return 1
	}

	fmt.Printf("settings saved to %s\n", cfg.AgentSettingsPath)
	fmt.Println("restart labtether-agent service for restart-required settings.")
	return 0
}

func runIdentityCommand(cfg RuntimeConfig, args []string) int {
	if len(args) > 0 {
		sub := strings.TrimSpace(strings.ToLower(args[0]))
		if sub == "help" || sub == "-h" || sub == "--help" {
			printIdentityHelp(cfg)
			return 0
		}
	}
	if len(args) == 0 || strings.TrimSpace(strings.ToLower(args[0])) == "show" {
		identity, err := settingsCLIEnsureDeviceIdentity(cfg)
		if err != nil {
			fmt.Printf("failed to load device identity: %v\n", err)
			return 1
		}
		hostname, _ := os.Hostname()
		fmt.Printf("Hostname: %s\n", strings.TrimSpace(hostname))
		fmt.Printf("Fingerprint: %s\n", identity.Fingerprint)
		fmt.Printf("Key Algorithm: %s\n", identity.KeyAlgorithm)
		fmt.Printf("Private Key: %s\n", cfg.DeviceKeyPath)
		fmt.Printf("Public Key: %s\n", cfg.DevicePublicKeyPath)
		fmt.Printf("Fingerprint File: %s\n", cfg.DeviceFingerprintPath)
		return 0
	}
	fmt.Printf("unknown identity command: %s\n", args[0])
	return 1
}

func runUpdateCommand(cfg RuntimeConfig, args []string) int {
	if len(args) == 0 {
		printUpdateHelp()
		return 0
	}

	switch strings.TrimSpace(strings.ToLower(args[0])) {
	case "help", "-h", "--help":
		printUpdateHelp()
		return 0
	case "self":
		force := false
		for _, raw := range args[1:] {
			arg := strings.TrimSpace(strings.ToLower(raw))
			switch arg {
			case "--force", "-f":
				force = true
			default:
				fmt.Printf("unknown update option: %s\n", raw)
				fmt.Println("usage: labtether-agent update self [--force]")
				return 1
			}
		}

		updated, summary, err := settingsCLISelfUpdate(cfg, selfUpdateOptions{Force: force})
		if err != nil {
			fmt.Printf("update failed: %v\n", err)
			return 1
		}
		fmt.Println(summary)
		if updated {
			fmt.Println("restart labtether-agent service to apply the updated binary to the running daemon.")
		}
		return 0
	default:
		fmt.Printf("unknown update command: %s\n", args[0])
		fmt.Println("usage: labtether-agent update self [--force]")
		return 1
	}
}

func promptWithDefault(reader *bufio.Reader, label, defaultValue string) string {
	defaultValue = strings.TrimSpace(defaultValue)
	if defaultValue == "" {
		fmt.Printf("%s: ", label)
	} else {
		fmt.Printf("%s [%s]: ", label, defaultValue)
	}
	input, err := reader.ReadString('\n')
	if err != nil {
		return defaultValue
	}
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return defaultValue
	}
	return trimmed
}
