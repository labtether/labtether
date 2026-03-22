package agentcore

import "time"

// TelemetrySample is the canonical endpoint-helper sample model shared across platform providers.
type TelemetrySample struct {
	AssetID          string    `json:"asset_id"`
	CPUPercent       float64   `json:"cpu_percent"`
	MemoryPercent    float64   `json:"memory_percent"`
	DiskPercent      float64   `json:"disk_percent"`
	NetRXBytes       float64   `json:"net_rx_bytes"`
	NetTXBytes       float64   `json:"net_tx_bytes"`
	NetRXBytesPerSec float64   `json:"net_rx_bytes_per_sec"`
	NetTXBytesPerSec float64   `json:"net_tx_bytes_per_sec"`
	TempCelsius      *float64  `json:"temp_celsius,omitempty"`
	CollectedAt      time.Time `json:"collected_at"`
}

// AgentInfo is the public health/info payload served by endpoint-helpers.
type AgentInfo struct {
	OS     string `json:"os"`
	Mode   string `json:"mode"`
	Status string `json:"status"`
}

// RuntimeConfig configures shared endpoint-helper runtime behavior.
type RuntimeConfig struct {
	Name                                      string
	Port                                      string
	APIBaseURL                                string
	APIToken                                  string // #nosec G117 -- Runtime API token, not a hardcoded credential.
	WSBaseURL                                 string
	AssetID                                   string
	GroupID                                   string
	Source                                    string
	CollectInterval                           time.Duration
	HeartbeatInterval                         time.Duration
	WebServiceDiscoveryInterval               time.Duration // LABTETHER_SERVICES_DISCOVERY_INTERVAL — web-service discovery loop interval
	LowPowerMode                              bool          // LABTETHER_LOW_POWER_MODE — apply low-power defaults for reduced CPU/memory footprint
	LogStreamEnabled                          bool          // LABTETHER_LOG_STREAM_ENABLED — enable continuous system log streaming
	EnrollmentToken                           string        // LABTETHER_ENROLLMENT_TOKEN — used for auto-enrollment
	EnrollmentTokenFilePath                   string        // LABTETHER_ENROLLMENT_TOKEN_FILE — optional file-based auto-enrollment token source
	TokenFilePath                             string        // LABTETHER_TOKEN_FILE — path to persist agent token
	DeviceKeyPath                             string        // LABTETHER_DEVICE_KEY_FILE — device private key path
	DevicePublicKeyPath                       string        // LABTETHER_DEVICE_PUBLIC_KEY_FILE — device public key path
	DeviceFingerprintPath                     string        // LABTETHER_DEVICE_FINGERPRINT_FILE — human-visible device fingerprint
	TLSCAFile                                 string        // LABTETHER_TLS_CA_FILE — custom CA cert for self-signed hub
	TLSSkipVerify                             bool          // LABTETHER_TLS_SKIP_VERIFY — skip cert verification (dev only)
	Version                                   string        // Agent version string (set at build time or from module info)
	DockerSocket                              string        // LABTETHER_DOCKER_SOCKET — Docker socket path for auto-detection
	DockerEnabled                             string        // LABTETHER_DOCKER_ENABLED — auto|true|false
	DockerDiscoveryInterval                   time.Duration // LABTETHER_DOCKER_DISCOVERY_INTERVAL — docker discovery/stats interval
	FileRootMode                              string        // LABTETHER_FILES_ROOT_MODE — home|full filesystem access for file operations
	WebRTCEnabled                             bool          // LABTETHER_WEBRTC_ENABLED — enable WebRTC streaming
	WebRTCSTUNURL                             string        // LABTETHER_WEBRTC_STUN_URL — STUN URL
	WebRTCTURNURL                             string        // LABTETHER_WEBRTC_TURN_URL — optional TURN URL
	WebRTCTURNUser                            string        // LABTETHER_WEBRTC_TURN_USER — optional TURN username
	WebRTCTURNPass                            string        // LABTETHER_WEBRTC_TURN_PASS — optional TURN password
	WebRTCTURNPassFilePath                    string        // LABTETHER_WEBRTC_TURN_PASS_FILE — optional file-based TURN password source
	WebRTCWaylandExperimentalEnabled          bool          // LABTETHER_WEBRTC_WAYLAND_EXPERIMENTAL_ENABLED — enable experimental Wayland desktop capture
	WebRTCWaylandPipeWireNodeID               string        // LABTETHER_WEBRTC_WAYLAND_PIPEWIRE_NODE_ID — PipeWire node ID for experimental Wayland capture
	WebRTCWaylandInputBackend                 string        // LABTETHER_WEBRTC_WAYLAND_INPUT_BACKEND — input injector backend for experimental Wayland capture
	CaptureFPS                                int           // LABTETHER_CAPTURE_FPS — default capture FPS for WebRTC streams
	AllowRemoteOverrides                      bool          // LABTETHER_ALLOW_REMOTE_OVERRIDES — allow hub-side settings apply
	AgentSettingsPath                         string        // LABTETHER_AGENT_SETTINGS_FILE — persisted local settings JSON
	LogLevel                                  string        // LABTETHER_LOG_LEVEL — runtime log level hint
	ServicesDiscoveryDockerEnabled            bool          // LABTETHER_SERVICES_DISCOVERY_DOCKER_ENABLED — include Docker-backed service discovery
	ServicesDiscoveryProxyEnabled             bool          // LABTETHER_SERVICES_DISCOVERY_PROXY_ENABLED — include reverse-proxy API discovery
	ServicesDiscoveryProxyTraefikEnabled      bool          // LABTETHER_SERVICES_DISCOVERY_PROXY_TRAEFIK_ENABLED — include Traefik API discovery
	ServicesDiscoveryProxyCaddyEnabled        bool          // LABTETHER_SERVICES_DISCOVERY_PROXY_CADDY_ENABLED — include Caddy API discovery
	ServicesDiscoveryProxyNPMEnabled          bool          // LABTETHER_SERVICES_DISCOVERY_PROXY_NPM_ENABLED — include Nginx Proxy Manager API discovery
	ServicesDiscoveryPortScanEnabled          bool          // LABTETHER_SERVICES_DISCOVERY_PORT_SCAN_ENABLED — include local host port scanning
	ServicesDiscoveryPortScanIncludeListening bool          // LABTETHER_SERVICES_DISCOVERY_PORT_SCAN_INCLUDE_LISTENING — augment local scan with listening ports
	ServicesDiscoveryPortScanPorts            string        // LABTETHER_SERVICES_DISCOVERY_PORT_SCAN_PORTS — optional local scan port allowlist
	ServicesDiscoveryLANScanEnabled           bool          // LABTETHER_SERVICES_DISCOVERY_LAN_SCAN_ENABLED — include LAN CIDR scanning
	ServicesDiscoveryLANScanCIDRs             string        // LABTETHER_SERVICES_DISCOVERY_LAN_SCAN_CIDRS — comma/newline separated CIDR list
	ServicesDiscoveryLANScanPorts             string        // LABTETHER_SERVICES_DISCOVERY_LAN_SCAN_PORTS — optional LAN scan port allowlist
	ServicesDiscoveryLANScanMaxHosts          int           // LABTETHER_SERVICES_DISCOVERY_LAN_SCAN_MAX_HOSTS — hard cap for LAN hosts per cycle
	AutoUpdateEnabled                         bool          // LABTETHER_AUTO_UPDATE — auto-check and apply agent binary updates on startup
	AutoUpdateCheckURL                        string        // LABTETHER_AUTO_UPDATE_CHECK_URL — optional release metadata override URL
}
