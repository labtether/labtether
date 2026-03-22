package agentcore

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

func LoadConfig(defaultName, defaultPort, defaultSource string) RuntimeConfig {
	hostname, _ := os.Hostname()
	assetID := strings.TrimSpace(envOrDefault("AGENT_ASSET_ID", ""))
	if assetID == "" {
		assetID = strings.TrimSpace(hostname)
	}
	if assetID == "" {
		assetID = strings.TrimSpace(defaultName) + "-local"
	}
	if assetID == "-local" {
		assetID = "labtether-agent-local"
	}

	source := strings.TrimSpace(envOrDefault("AGENT_SOURCE", defaultSource))
	if source == "" {
		source = defaultSource
	}

	tokenFile := strings.TrimSpace(envOrDefault("LABTETHER_TOKEN_FILE", defaultTokenFile))
	enrollmentTokenFile := strings.TrimSpace(envOrDefault("LABTETHER_ENROLLMENT_TOKEN_FILE", ""))
	settingsFile := strings.TrimSpace(envOrDefault("LABTETHER_AGENT_SETTINGS_FILE", defaultSettingsFile))
	deviceKeyFile := strings.TrimSpace(envOrDefault("LABTETHER_DEVICE_KEY_FILE", defaultDeviceKeyFile))
	devicePublicKeyFile := strings.TrimSpace(envOrDefault("LABTETHER_DEVICE_PUBLIC_KEY_FILE", defaultDevicePublicKeyFile))
	deviceFingerprintFile := strings.TrimSpace(envOrDefault("LABTETHER_DEVICE_FINGERPRINT_FILE", defaultDeviceFingerprintFile))
	turnPassFile := strings.TrimSpace(envOrDefault("LABTETHER_WEBRTC_TURN_PASS_FILE", ""))
	allowRemoteOverrides := true
	if raw := strings.TrimSpace(os.Getenv("LABTETHER_ALLOW_REMOTE_OVERRIDES")); raw != "" {
		allowRemoteOverrides = !(strings.EqualFold(raw, "false") || raw == "0" || strings.EqualFold(raw, "off"))
	}
	autoUpdateEnabled := true
	if raw := strings.TrimSpace(os.Getenv("LABTETHER_AUTO_UPDATE")); raw != "" {
		autoUpdateEnabled = !(strings.EqualFold(raw, "false") || raw == "0" || strings.EqualFold(raw, "off"))
	}
	webRTCEnabled := true
	if raw := strings.TrimSpace(os.Getenv("LABTETHER_WEBRTC_ENABLED")); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			webRTCEnabled = parsed
		}
	}
	webRTCWaylandExperimentalEnabled := parseBoolEnv("LABTETHER_WEBRTC_WAYLAND_EXPERIMENTAL_ENABLED", false)
	webRTCWaylandPipeWireNodeID := strings.TrimSpace(envOrDefault("LABTETHER_WEBRTC_WAYLAND_PIPEWIRE_NODE_ID", ""))
	webRTCWaylandInputBackend := strings.TrimSpace(strings.ToLower(envOrDefault("LABTETHER_WEBRTC_WAYLAND_INPUT_BACKEND", "auto")))
	captureFPS := 30
	if raw := strings.TrimSpace(os.Getenv("LABTETHER_CAPTURE_FPS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			captureFPS = parsed
		}
	}
	lowPowerMode := parseBoolEnv("LABTETHER_LOW_POWER_MODE", false)
	collectIntervalDefault := 10 * time.Second
	heartbeatIntervalDefault := 20 * time.Second
	dockerDiscoveryIntervalDefault := 30 * time.Second
	webServiceDiscoveryIntervalDefault := 60 * time.Second
	servicesDiscoveryPortScanEnabledDefault := true
	servicesDiscoveryPortScanIncludeListeningDefault := true
	logStreamEnabledDefault := true
	if lowPowerMode {
		// Ultra low-power profile: trade freshness for lower idle CPU/memory.
		collectIntervalDefault = 30 * time.Second
		heartbeatIntervalDefault = 120 * time.Second
		dockerDiscoveryIntervalDefault = 5 * time.Minute
		webServiceDiscoveryIntervalDefault = 10 * time.Minute
		servicesDiscoveryPortScanEnabledDefault = false
		servicesDiscoveryPortScanIncludeListeningDefault = false
		logStreamEnabledDefault = false
	}
	servicesDiscoveryDockerEnabled := parseBoolEnv("LABTETHER_SERVICES_DISCOVERY_DOCKER_ENABLED", true)
	servicesDiscoveryProxyEnabled := parseBoolEnv("LABTETHER_SERVICES_DISCOVERY_PROXY_ENABLED", true)
	if raw := strings.TrimSpace(os.Getenv("LABTETHER_PROXY_DISABLED")); raw != "" {
		servicesDiscoveryProxyEnabled = !(strings.EqualFold(raw, "true") || raw == "1" || strings.EqualFold(raw, "on"))
	}
	servicesDiscoveryProxyTraefikEnabled := parseBoolEnv("LABTETHER_SERVICES_DISCOVERY_PROXY_TRAEFIK_ENABLED", true)
	servicesDiscoveryProxyCaddyEnabled := parseBoolEnv("LABTETHER_SERVICES_DISCOVERY_PROXY_CADDY_ENABLED", true)
	servicesDiscoveryProxyNPMEnabled := parseBoolEnv("LABTETHER_SERVICES_DISCOVERY_PROXY_NPM_ENABLED", true)
	servicesDiscoveryPortScanEnabled := parseBoolEnv("LABTETHER_SERVICES_DISCOVERY_PORT_SCAN_ENABLED", servicesDiscoveryPortScanEnabledDefault)
	if raw := strings.TrimSpace(os.Getenv("LABTETHER_WEBSVC_PORTSCAN_DISABLED")); raw != "" {
		servicesDiscoveryPortScanEnabled = !(strings.EqualFold(raw, "true") || raw == "1" || strings.EqualFold(raw, "on"))
	}
	servicesDiscoveryPortScanIncludeListening := parseBoolEnv("LABTETHER_SERVICES_DISCOVERY_PORT_SCAN_INCLUDE_LISTENING", servicesDiscoveryPortScanIncludeListeningDefault)
	if raw := strings.TrimSpace(os.Getenv("LABTETHER_WEBSVC_PORTSCAN_INCLUDE_LISTENING")); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			servicesDiscoveryPortScanIncludeListening = parsed
		}
	}
	servicesDiscoveryPortScanPorts := strings.TrimSpace(envOrDefault("LABTETHER_SERVICES_DISCOVERY_PORT_SCAN_PORTS", strings.TrimSpace(os.Getenv("LABTETHER_WEBSVC_PORTSCAN_PORTS"))))
	servicesDiscoveryLANScanEnabled := parseBoolEnv("LABTETHER_SERVICES_DISCOVERY_LAN_SCAN_ENABLED", false)
	servicesDiscoveryLANScanCIDRs := strings.TrimSpace(envOrDefault("LABTETHER_SERVICES_DISCOVERY_LAN_SCAN_CIDRS", ""))
	servicesDiscoveryLANScanPorts := strings.TrimSpace(envOrDefault("LABTETHER_SERVICES_DISCOVERY_LAN_SCAN_PORTS", ""))
	servicesDiscoveryLANScanMaxHosts := 64
	if raw := strings.TrimSpace(os.Getenv("LABTETHER_SERVICES_DISCOVERY_LAN_SCAN_MAX_HOSTS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			servicesDiscoveryLANScanMaxHosts = parsed
		}
	}
	logStreamEnabled := parseBoolEnv("LABTETHER_LOG_STREAM_ENABLED", logStreamEnabledDefault)

	apiToken := strings.TrimSpace(envOrDefault("LABTETHER_API_TOKEN", ""))
	if apiToken == "" {
		if token, err := loadTokenFromFile(tokenFile); err == nil {
			apiToken = strings.TrimSpace(token)
		}
	}
	enrollmentToken := strings.TrimSpace(envOrDefault("LABTETHER_ENROLLMENT_TOKEN", ""))
	if enrollmentToken == "" {
		if token, err := loadSecretFromFile(enrollmentTokenFile); err == nil {
			enrollmentToken = strings.TrimSpace(token)
		}
	}
	turnPass := strings.TrimSpace(envOrDefault("LABTETHER_WEBRTC_TURN_PASS", ""))
	if turnPass == "" {
		if token, err := loadSecretFromFile(turnPassFile); err == nil {
			turnPass = strings.TrimSpace(token)
		}
	}

	cfg := RuntimeConfig{
		Name:                                 strings.TrimSpace(envOrDefault("AGENT_NAME", defaultName)),
		Port:                                 strings.TrimSpace(envOrDefault("AGENT_PORT", defaultPort)),
		APIBaseURL:                           strings.TrimSpace(envOrDefault("LABTETHER_API_BASE_URL", "")),
		APIToken:                             apiToken,
		WSBaseURL:                            strings.TrimSpace(envOrDefault("LABTETHER_WS_URL", "")),
		AssetID:                              assetID,
		GroupID:                              strings.TrimSpace(envOrDefault("AGENT_GROUP_ID", "")),
		Source:                               source,
		CollectInterval:                      parseDurationOrDefault(os.Getenv("AGENT_COLLECT_INTERVAL"), collectIntervalDefault, 2*time.Second, 5*time.Minute),
		HeartbeatInterval:                    parseDurationOrDefault(os.Getenv("AGENT_HEARTBEAT_INTERVAL"), heartbeatIntervalDefault, 5*time.Second, 10*time.Minute),
		WebServiceDiscoveryInterval:          parseDurationOrDefault(os.Getenv("LABTETHER_SERVICES_DISCOVERY_INTERVAL"), webServiceDiscoveryIntervalDefault, 30*time.Second, 6*time.Hour),
		LowPowerMode:                         lowPowerMode,
		LogStreamEnabled:                     logStreamEnabled,
		EnrollmentToken:                      enrollmentToken,
		EnrollmentTokenFilePath:              enrollmentTokenFile,
		TokenFilePath:                        tokenFile,
		AgentSettingsPath:                    settingsFile,
		DeviceKeyPath:                        deviceKeyFile,
		DevicePublicKeyPath:                  devicePublicKeyFile,
		DeviceFingerprintPath:                deviceFingerprintFile,
		TLSCAFile:                            strings.TrimSpace(envOrDefault("LABTETHER_TLS_CA_FILE", "")),
		TLSSkipVerify:                        parseBoolEnv("LABTETHER_TLS_SKIP_VERIFY", false),
		Version:                              agentVersion(),
		DockerSocket:                         strings.TrimSpace(envOrDefault("LABTETHER_DOCKER_SOCKET", "/var/run/docker.sock")),
		DockerEnabled:                        strings.TrimSpace(strings.ToLower(envOrDefault("LABTETHER_DOCKER_ENABLED", "auto"))),
		DockerDiscoveryInterval:              parseDurationOrDefault(os.Getenv("LABTETHER_DOCKER_DISCOVERY_INTERVAL"), dockerDiscoveryIntervalDefault, 5*time.Second, time.Hour),
		FileRootMode:                         strings.TrimSpace(strings.ToLower(envOrDefault("LABTETHER_FILES_ROOT_MODE", "home"))),
		WebRTCEnabled:                        webRTCEnabled,
		WebRTCSTUNURL:                        strings.TrimSpace(envOrDefault("LABTETHER_WEBRTC_STUN_URL", "stun:stun.l.google.com:19302")),
		WebRTCTURNURL:                        strings.TrimSpace(envOrDefault("LABTETHER_WEBRTC_TURN_URL", "")),
		WebRTCTURNUser:                       strings.TrimSpace(envOrDefault("LABTETHER_WEBRTC_TURN_USER", "")),
		WebRTCTURNPass:                       turnPass,
		WebRTCTURNPassFilePath:               turnPassFile,
		WebRTCWaylandExperimentalEnabled:     webRTCWaylandExperimentalEnabled,
		WebRTCWaylandPipeWireNodeID:          webRTCWaylandPipeWireNodeID,
		WebRTCWaylandInputBackend:            webRTCWaylandInputBackend,
		CaptureFPS:                           captureFPS,
		AllowRemoteOverrides:                 allowRemoteOverrides,
		LogLevel:                             strings.TrimSpace(strings.ToLower(envOrDefault("LABTETHER_LOG_LEVEL", "info"))),
		ServicesDiscoveryDockerEnabled:       servicesDiscoveryDockerEnabled,
		ServicesDiscoveryProxyEnabled:        servicesDiscoveryProxyEnabled,
		ServicesDiscoveryProxyTraefikEnabled: servicesDiscoveryProxyTraefikEnabled,
		ServicesDiscoveryProxyCaddyEnabled:   servicesDiscoveryProxyCaddyEnabled,
		ServicesDiscoveryProxyNPMEnabled:     servicesDiscoveryProxyNPMEnabled,
		ServicesDiscoveryPortScanEnabled:     servicesDiscoveryPortScanEnabled,
		ServicesDiscoveryPortScanIncludeListening: servicesDiscoveryPortScanIncludeListening,
		ServicesDiscoveryPortScanPorts:            servicesDiscoveryPortScanPorts,
		ServicesDiscoveryLANScanEnabled:           servicesDiscoveryLANScanEnabled,
		ServicesDiscoveryLANScanCIDRs:             servicesDiscoveryLANScanCIDRs,
		ServicesDiscoveryLANScanPorts:             servicesDiscoveryLANScanPorts,
		ServicesDiscoveryLANScanMaxHosts:          servicesDiscoveryLANScanMaxHosts,
		AutoUpdateEnabled:                         autoUpdateEnabled,
		AutoUpdateCheckURL:                        strings.TrimSpace(envOrDefault("LABTETHER_AUTO_UPDATE_CHECK_URL", "")),
	}

	if cfg.DockerEnabled == "" {
		cfg.DockerEnabled = "auto"
	}
	if _, ok := AgentSettingDefinitionByKey(SettingKeyDockerEnabled); ok {
		if normalized, err := NormalizeAgentSettingValue(SettingKeyDockerEnabled, cfg.DockerEnabled); err == nil {
			cfg.DockerEnabled = normalized
		}
	}
	if normalized, err := NormalizeAgentSettingValue(SettingKeyFilesRootMode, cfg.FileRootMode); err == nil {
		cfg.FileRootMode = normalized
	}
	if normalized, err := NormalizeAgentSettingValue(SettingKeyLogLevel, cfg.LogLevel); err == nil {
		cfg.LogLevel = normalized
	}

	if settings, err := LoadAgentSettingsFile(cfg.AgentSettingsPath); err == nil && len(settings) > 0 {
		if raw, ok := settings[SettingKeyTLSCAFile]; ok {
			cfg.TLSCAFile = strings.TrimSpace(raw)
		}
		if raw, ok := settings[SettingKeyTLSSkipVerify]; ok {
			if enabled, err := strconv.ParseBool(strings.TrimSpace(raw)); err == nil {
				cfg.TLSSkipVerify = enabled
			}
		}
		if raw := strings.TrimSpace(settings[SettingKeyCollectIntervalSec]); raw != "" {
			if seconds, err := strconv.Atoi(raw); err == nil {
				cfg.CollectInterval = time.Duration(seconds) * time.Second
			}
		}
		if raw := strings.TrimSpace(settings[SettingKeyHeartbeatIntervalSec]); raw != "" {
			if seconds, err := strconv.Atoi(raw); err == nil {
				cfg.HeartbeatInterval = time.Duration(seconds) * time.Second
			}
		}
		if raw := strings.TrimSpace(settings[SettingKeyDockerEnabled]); raw != "" {
			cfg.DockerEnabled = raw
		}
		if raw := strings.TrimSpace(settings[SettingKeyDockerEndpoint]); raw != "" {
			cfg.DockerSocket = raw
		}
		if raw := strings.TrimSpace(settings[SettingKeyDockerDiscoveryIntervalSec]); raw != "" {
			if seconds, err := strconv.Atoi(raw); err == nil {
				cfg.DockerDiscoveryInterval = time.Duration(seconds) * time.Second
			}
		}
		if raw := strings.TrimSpace(settings[SettingKeyServicesDiscoveryDockerEnabled]); raw != "" {
			if enabled, err := strconv.ParseBool(raw); err == nil {
				cfg.ServicesDiscoveryDockerEnabled = enabled
			}
		}
		if raw := strings.TrimSpace(settings[SettingKeyServicesDiscoveryProxyEnabled]); raw != "" {
			if enabled, err := strconv.ParseBool(raw); err == nil {
				cfg.ServicesDiscoveryProxyEnabled = enabled
			}
		}
		if raw := strings.TrimSpace(settings[SettingKeyServicesDiscoveryProxyTraefikEnabled]); raw != "" {
			if enabled, err := strconv.ParseBool(raw); err == nil {
				cfg.ServicesDiscoveryProxyTraefikEnabled = enabled
			}
		}
		if raw := strings.TrimSpace(settings[SettingKeyServicesDiscoveryProxyCaddyEnabled]); raw != "" {
			if enabled, err := strconv.ParseBool(raw); err == nil {
				cfg.ServicesDiscoveryProxyCaddyEnabled = enabled
			}
		}
		if raw := strings.TrimSpace(settings[SettingKeyServicesDiscoveryProxyNPMEnabled]); raw != "" {
			if enabled, err := strconv.ParseBool(raw); err == nil {
				cfg.ServicesDiscoveryProxyNPMEnabled = enabled
			}
		}
		if raw := strings.TrimSpace(settings[SettingKeyServicesDiscoveryPortScanEnabled]); raw != "" {
			if enabled, err := strconv.ParseBool(raw); err == nil {
				cfg.ServicesDiscoveryPortScanEnabled = enabled
			}
		}
		if raw := strings.TrimSpace(settings[SettingKeyServicesDiscoveryPortScanIncludeListening]); raw != "" {
			if enabled, err := strconv.ParseBool(raw); err == nil {
				cfg.ServicesDiscoveryPortScanIncludeListening = enabled
			}
		}
		if raw, ok := settings[SettingKeyServicesDiscoveryPortScanPorts]; ok {
			cfg.ServicesDiscoveryPortScanPorts = strings.TrimSpace(raw)
		}
		if raw := strings.TrimSpace(settings[SettingKeyServicesDiscoveryLANScanEnabled]); raw != "" {
			if enabled, err := strconv.ParseBool(raw); err == nil {
				cfg.ServicesDiscoveryLANScanEnabled = enabled
			}
		}
		if raw, ok := settings[SettingKeyServicesDiscoveryLANScanCIDRs]; ok {
			cfg.ServicesDiscoveryLANScanCIDRs = strings.TrimSpace(raw)
		}
		if raw, ok := settings[SettingKeyServicesDiscoveryLANScanPorts]; ok {
			cfg.ServicesDiscoveryLANScanPorts = strings.TrimSpace(raw)
		}
		if raw := strings.TrimSpace(settings[SettingKeyServicesDiscoveryLANScanMaxHosts]); raw != "" {
			if maxHosts, err := strconv.Atoi(raw); err == nil {
				cfg.ServicesDiscoveryLANScanMaxHosts = maxHosts
			}
		}
		if raw := strings.TrimSpace(settings[SettingKeyFilesRootMode]); raw != "" {
			cfg.FileRootMode = raw
		}
		if raw := strings.TrimSpace(settings[SettingKeyWebRTCEnabled]); raw != "" {
			if enabled, err := strconv.ParseBool(raw); err == nil {
				cfg.WebRTCEnabled = enabled
			}
		}
		if raw, ok := settings[SettingKeyWebRTCSTUNURL]; ok {
			cfg.WebRTCSTUNURL = strings.TrimSpace(raw)
		}
		if raw, ok := settings[SettingKeyWebRTCTURNURL]; ok {
			cfg.WebRTCTURNURL = strings.TrimSpace(raw)
		}
		if raw, ok := settings[SettingKeyWebRTCTURNUser]; ok {
			cfg.WebRTCTURNUser = strings.TrimSpace(raw)
		}
		if raw, ok := settings[SettingKeyWebRTCTURNPass]; ok {
			cfg.WebRTCTURNPass = strings.TrimSpace(raw)
		}
		if raw := strings.TrimSpace(settings[SettingKeyWebRTCWaylandExperimentalEnabled]); raw != "" {
			if enabled, err := strconv.ParseBool(raw); err == nil {
				cfg.WebRTCWaylandExperimentalEnabled = enabled
			}
		}
		if raw, ok := settings[SettingKeyWebRTCWaylandPipeWireNodeID]; ok {
			cfg.WebRTCWaylandPipeWireNodeID = strings.TrimSpace(raw)
		}
		if raw, ok := settings[SettingKeyWebRTCWaylandInputBackend]; ok {
			cfg.WebRTCWaylandInputBackend = strings.TrimSpace(strings.ToLower(raw))
		}
		if raw := strings.TrimSpace(settings[SettingKeyCaptureFPS]); raw != "" {
			if fps, err := strconv.Atoi(raw); err == nil {
				cfg.CaptureFPS = fps
			}
		}
		if raw := strings.TrimSpace(settings[SettingKeyAllowRemoteOverrides]); raw != "" {
			if enabled, err := strconv.ParseBool(raw); err == nil {
				cfg.AllowRemoteOverrides = enabled
			}
		}
		if raw := strings.TrimSpace(settings[SettingKeyLogLevel]); raw != "" {
			cfg.LogLevel = raw
		}
	}
	if normalized, err := NormalizeAgentSettingValue(SettingKeyFilesRootMode, cfg.FileRootMode); err == nil {
		cfg.FileRootMode = normalized
	} else {
		cfg.FileRootMode = "home"
	}
	if normalized, err := NormalizeAgentSettingValue(SettingKeyCaptureFPS, strconv.Itoa(cfg.CaptureFPS)); err == nil {
		if fps, convErr := strconv.Atoi(normalized); convErr == nil {
			cfg.CaptureFPS = fps
		}
	} else {
		cfg.CaptureFPS = 30
	}
	if normalized, err := NormalizeAgentSettingValue(SettingKeyWebRTCWaylandExperimentalEnabled, strconv.FormatBool(cfg.WebRTCWaylandExperimentalEnabled)); err == nil {
		cfg.WebRTCWaylandExperimentalEnabled = strings.EqualFold(normalized, "true")
	} else {
		cfg.WebRTCWaylandExperimentalEnabled = false
	}
	if normalized, err := NormalizeAgentSettingValue(SettingKeyWebRTCWaylandPipeWireNodeID, cfg.WebRTCWaylandPipeWireNodeID); err == nil {
		cfg.WebRTCWaylandPipeWireNodeID = normalized
	} else {
		cfg.WebRTCWaylandPipeWireNodeID = ""
	}
	if normalized, err := NormalizeAgentSettingValue(SettingKeyWebRTCWaylandInputBackend, cfg.WebRTCWaylandInputBackend); err == nil {
		cfg.WebRTCWaylandInputBackend = normalized
	} else {
		cfg.WebRTCWaylandInputBackend = "auto"
	}
	if normalized, err := NormalizeAgentSettingValue(SettingKeyServicesDiscoveryDockerEnabled, strconv.FormatBool(cfg.ServicesDiscoveryDockerEnabled)); err == nil {
		cfg.ServicesDiscoveryDockerEnabled = strings.EqualFold(normalized, "true")
	} else {
		cfg.ServicesDiscoveryDockerEnabled = true
	}
	if normalized, err := NormalizeAgentSettingValue(SettingKeyServicesDiscoveryProxyEnabled, strconv.FormatBool(cfg.ServicesDiscoveryProxyEnabled)); err == nil {
		cfg.ServicesDiscoveryProxyEnabled = strings.EqualFold(normalized, "true")
	} else {
		cfg.ServicesDiscoveryProxyEnabled = true
	}
	if normalized, err := NormalizeAgentSettingValue(SettingKeyServicesDiscoveryProxyTraefikEnabled, strconv.FormatBool(cfg.ServicesDiscoveryProxyTraefikEnabled)); err == nil {
		cfg.ServicesDiscoveryProxyTraefikEnabled = strings.EqualFold(normalized, "true")
	} else {
		cfg.ServicesDiscoveryProxyTraefikEnabled = true
	}
	if normalized, err := NormalizeAgentSettingValue(SettingKeyServicesDiscoveryProxyCaddyEnabled, strconv.FormatBool(cfg.ServicesDiscoveryProxyCaddyEnabled)); err == nil {
		cfg.ServicesDiscoveryProxyCaddyEnabled = strings.EqualFold(normalized, "true")
	} else {
		cfg.ServicesDiscoveryProxyCaddyEnabled = true
	}
	if normalized, err := NormalizeAgentSettingValue(SettingKeyServicesDiscoveryProxyNPMEnabled, strconv.FormatBool(cfg.ServicesDiscoveryProxyNPMEnabled)); err == nil {
		cfg.ServicesDiscoveryProxyNPMEnabled = strings.EqualFold(normalized, "true")
	} else {
		cfg.ServicesDiscoveryProxyNPMEnabled = true
	}
	if normalized, err := NormalizeAgentSettingValue(SettingKeyServicesDiscoveryPortScanEnabled, strconv.FormatBool(cfg.ServicesDiscoveryPortScanEnabled)); err == nil {
		cfg.ServicesDiscoveryPortScanEnabled = strings.EqualFold(normalized, "true")
	} else {
		cfg.ServicesDiscoveryPortScanEnabled = true
	}
	if normalized, err := NormalizeAgentSettingValue(SettingKeyServicesDiscoveryPortScanIncludeListening, strconv.FormatBool(cfg.ServicesDiscoveryPortScanIncludeListening)); err == nil {
		cfg.ServicesDiscoveryPortScanIncludeListening = strings.EqualFold(normalized, "true")
	} else {
		cfg.ServicesDiscoveryPortScanIncludeListening = true
	}
	if normalized, err := NormalizeAgentSettingValue(SettingKeyServicesDiscoveryPortScanPorts, cfg.ServicesDiscoveryPortScanPorts); err == nil {
		cfg.ServicesDiscoveryPortScanPorts = normalized
	} else {
		cfg.ServicesDiscoveryPortScanPorts = ""
	}
	if normalized, err := NormalizeAgentSettingValue(SettingKeyServicesDiscoveryLANScanEnabled, strconv.FormatBool(cfg.ServicesDiscoveryLANScanEnabled)); err == nil {
		cfg.ServicesDiscoveryLANScanEnabled = strings.EqualFold(normalized, "true")
	} else {
		cfg.ServicesDiscoveryLANScanEnabled = false
	}
	if normalized, err := NormalizeAgentSettingValue(SettingKeyServicesDiscoveryLANScanCIDRs, cfg.ServicesDiscoveryLANScanCIDRs); err == nil {
		cfg.ServicesDiscoveryLANScanCIDRs = normalized
	} else {
		cfg.ServicesDiscoveryLANScanCIDRs = ""
	}
	if normalized, err := NormalizeAgentSettingValue(SettingKeyServicesDiscoveryLANScanPorts, cfg.ServicesDiscoveryLANScanPorts); err == nil {
		cfg.ServicesDiscoveryLANScanPorts = normalized
	} else {
		cfg.ServicesDiscoveryLANScanPorts = ""
	}
	if normalized, err := NormalizeAgentSettingValue(SettingKeyServicesDiscoveryLANScanMaxHosts, strconv.Itoa(cfg.ServicesDiscoveryLANScanMaxHosts)); err == nil {
		if parsed, convErr := strconv.Atoi(normalized); convErr == nil {
			cfg.ServicesDiscoveryLANScanMaxHosts = parsed
		}
	} else {
		cfg.ServicesDiscoveryLANScanMaxHosts = 64
	}
	if normalized, err := NormalizeAgentSettingValue(SettingKeyDockerEndpoint, cfg.DockerSocket); err == nil {
		cfg.DockerSocket = normalized
	} else {
		cfg.DockerSocket = "/var/run/docker.sock"
	}

	// Auto-load CA from enrollment if not explicitly configured
	if cfg.TLSCAFile == "" {
		savedCA := filepath.Join(filepath.Dir(cfg.TokenFilePath), "ca.crt")
		if _, err := os.Stat(savedCA); err == nil {
			cfg.TLSCAFile = savedCA
		}
	}
	cfg.APIBaseURL = normalizeAPIBaseURL(cfg.APIBaseURL)
	cfg.WSBaseURL = normalizeWSBaseURL(cfg.WSBaseURL)
	cfg.AutoUpdateCheckURL = normalizeHTTPSURL(cfg.AutoUpdateCheckURL)

	return cfg
}

func loadSecretFromFile(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	data, err := os.ReadFile(path) // #nosec G304,G703 -- Config path is runtime configuration/default state, not untrusted user input.
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func agentVersion() string {
	if v := os.Getenv("LABTETHER_AGENT_VERSION"); v != "" {
		return v
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if derived := deriveAgentVersionFromBuildInfo(info.Main.Version, info.Settings); derived != "" {
			return derived
		}
	}
	return "dev"
}

func deriveAgentVersionFromBuildInfo(mainVersion string, settings []debug.BuildSetting) string {
	version := strings.TrimSpace(mainVersion)
	if version != "" && version != "(devel)" {
		return version
	}

	revision := ""
	modified := false
	for _, setting := range settings {
		switch strings.TrimSpace(setting.Key) {
		case "vcs.revision":
			revision = strings.TrimSpace(setting.Value)
		case "vcs.modified":
			modified = strings.EqualFold(strings.TrimSpace(setting.Value), "true")
		}
	}
	if revision == "" {
		return ""
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if modified {
		revision += "-dirty"
	}
	return "git:" + revision
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func parseDurationOrDefault(raw string, fallback, minValue, maxValue time.Duration) time.Duration {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback
	}
	if seconds, err := strconv.Atoi(trimmed); err == nil {
		duration := time.Duration(seconds) * time.Second
		if duration < minValue {
			return minValue
		}
		if duration > maxValue {
			return maxValue
		}
		return duration
	}
	duration, err := time.ParseDuration(trimmed)
	if err != nil {
		return fallback
	}
	if duration < minValue {
		return minValue
	}
	if duration > maxValue {
		return maxValue
	}
	return duration
}

func parseBoolEnv(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

const envAllowInsecureTransport = "LABTETHER_ALLOW_INSECURE_TRANSPORT"

func allowInsecureTransportOptIn() bool {
	return parseBoolEnv(envAllowInsecureTransport, false)
}

func normalizeAPIBaseURL(raw string) string {
	return normalizeURLScheme(raw, "https", "http", map[string]string{
		"wss": "https",
		"ws":  "http",
	})
}

func normalizeWSBaseURL(raw string) string {
	return normalizeURLScheme(raw, "wss", "ws", map[string]string{
		"https": "wss",
		"http":  "ws",
	})
}

func normalizeHTTPSURL(raw string) string {
	return normalizeURLScheme(raw, "https", "http", nil)
}

func normalizeURLScheme(raw, secureScheme, insecureScheme string, aliases map[string]string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || strings.TrimSpace(parsed.Scheme) == "" {
		return trimmed
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if aliases != nil {
		if mapped, ok := aliases[scheme]; ok {
			scheme = mapped
		}
	}
	switch scheme {
	case secureScheme:
		parsed.Scheme = secureScheme
	case insecureScheme:
		if allowInsecureTransportOptIn() {
			parsed.Scheme = insecureScheme
		} else {
			parsed.Scheme = secureScheme
		}
	}
	return parsed.String()
}
