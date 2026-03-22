package sysconfig

import (
	"strconv"
	"strings"
	"time"
)

// ConfigSnapshot holds the subset of runtime configuration fields needed to
// compute agent setting values. The parent agentcore package populates this
// from its RuntimeConfig struct.
type ConfigSnapshot struct {
	CollectInterval                           time.Duration
	HeartbeatInterval                         time.Duration
	DockerEnabled                             string
	DockerSocket                              string
	DockerDiscoveryInterval                   time.Duration
	ServicesDiscoveryDockerEnabled            bool
	ServicesDiscoveryProxyEnabled             bool
	ServicesDiscoveryProxyTraefikEnabled      bool
	ServicesDiscoveryProxyCaddyEnabled        bool
	ServicesDiscoveryProxyNPMEnabled          bool
	ServicesDiscoveryPortScanEnabled          bool
	ServicesDiscoveryPortScanIncludeListening bool
	ServicesDiscoveryPortScanPorts            string
	ServicesDiscoveryLANScanEnabled           bool
	ServicesDiscoveryLANScanCIDRs             string
	ServicesDiscoveryLANScanPorts             string
	ServicesDiscoveryLANScanMaxHosts          int
	FileRootMode                              string
	WebRTCEnabled                             bool
	WebRTCSTUNURL                             string
	WebRTCTURNURL                             string
	WebRTCTURNUser                            string
	WebRTCTURNPass                            string
	WebRTCWaylandExperimentalEnabled          bool
	WebRTCWaylandPipeWireNodeID               string
	WebRTCWaylandInputBackend                 string
	CaptureFPS                                int
	TLSSkipVerify                             bool
	TLSCAFile                                 string
	AllowRemoteOverrides                      bool
	LogLevel                                  string
}

func AgentSettingValuesFromConfig(cfg ConfigSnapshot) map[string]string {
	values := DefaultAgentSettingValues()

	values[SettingKeyCollectIntervalSec] = strconv.Itoa(int(DurationSeconds(cfg.CollectInterval)))
	values[SettingKeyHeartbeatIntervalSec] = strconv.Itoa(int(DurationSeconds(cfg.HeartbeatInterval)))

	dockerEnabled := strings.TrimSpace(strings.ToLower(cfg.DockerEnabled))
	if dockerEnabled == "" {
		dockerEnabled = "auto"
	}
	values[SettingKeyDockerEnabled] = dockerEnabled

	dockerEndpoint := strings.TrimSpace(cfg.DockerSocket)
	if dockerEndpoint == "" {
		dockerEndpoint = "/var/run/docker.sock"
	}
	values[SettingKeyDockerEndpoint] = dockerEndpoint

	discoveryInterval := cfg.DockerDiscoveryInterval
	if discoveryInterval <= 0 {
		discoveryInterval = 30 * time.Second
	}
	values[SettingKeyDockerDiscoveryIntervalSec] = strconv.Itoa(int(DurationSeconds(discoveryInterval)))

	values[SettingKeyServicesDiscoveryDockerEnabled] = strconv.FormatBool(cfg.ServicesDiscoveryDockerEnabled)
	values[SettingKeyServicesDiscoveryProxyEnabled] = strconv.FormatBool(cfg.ServicesDiscoveryProxyEnabled)
	values[SettingKeyServicesDiscoveryProxyTraefikEnabled] = strconv.FormatBool(cfg.ServicesDiscoveryProxyTraefikEnabled)
	values[SettingKeyServicesDiscoveryProxyCaddyEnabled] = strconv.FormatBool(cfg.ServicesDiscoveryProxyCaddyEnabled)
	values[SettingKeyServicesDiscoveryProxyNPMEnabled] = strconv.FormatBool(cfg.ServicesDiscoveryProxyNPMEnabled)
	values[SettingKeyServicesDiscoveryPortScanEnabled] = strconv.FormatBool(cfg.ServicesDiscoveryPortScanEnabled)
	values[SettingKeyServicesDiscoveryPortScanIncludeListening] = strconv.FormatBool(cfg.ServicesDiscoveryPortScanIncludeListening)
	values[SettingKeyServicesDiscoveryPortScanPorts] = strings.TrimSpace(cfg.ServicesDiscoveryPortScanPorts)
	values[SettingKeyServicesDiscoveryLANScanEnabled] = strconv.FormatBool(cfg.ServicesDiscoveryLANScanEnabled)
	values[SettingKeyServicesDiscoveryLANScanCIDRs] = strings.TrimSpace(cfg.ServicesDiscoveryLANScanCIDRs)
	values[SettingKeyServicesDiscoveryLANScanPorts] = strings.TrimSpace(cfg.ServicesDiscoveryLANScanPorts)

	lanMaxHosts := cfg.ServicesDiscoveryLANScanMaxHosts
	if lanMaxHosts <= 0 {
		lanMaxHosts = 64
	}
	values[SettingKeyServicesDiscoveryLANScanMaxHosts] = strconv.Itoa(lanMaxHosts)

	fileRootMode := strings.TrimSpace(strings.ToLower(cfg.FileRootMode))
	if fileRootMode == "" {
		fileRootMode = "home"
	}
	values[SettingKeyFilesRootMode] = fileRootMode
	values[SettingKeyWebRTCEnabled] = strconv.FormatBool(cfg.WebRTCEnabled)

	stunURL := strings.TrimSpace(cfg.WebRTCSTUNURL)
	if stunURL == "" {
		stunURL = "stun:stun.l.google.com:19302"
	}
	values[SettingKeyWebRTCSTUNURL] = stunURL
	values[SettingKeyWebRTCTURNURL] = strings.TrimSpace(cfg.WebRTCTURNURL)
	values[SettingKeyWebRTCTURNUser] = strings.TrimSpace(cfg.WebRTCTURNUser)
	values[SettingKeyWebRTCTURNPass] = strings.TrimSpace(cfg.WebRTCTURNPass)
	values[SettingKeyWebRTCWaylandExperimentalEnabled] = strconv.FormatBool(cfg.WebRTCWaylandExperimentalEnabled)
	values[SettingKeyWebRTCWaylandPipeWireNodeID] = strings.TrimSpace(cfg.WebRTCWaylandPipeWireNodeID)
	waylandInputBackend := strings.TrimSpace(strings.ToLower(cfg.WebRTCWaylandInputBackend))
	if waylandInputBackend == "" {
		waylandInputBackend = "auto"
	}
	values[SettingKeyWebRTCWaylandInputBackend] = waylandInputBackend

	captureFPS := cfg.CaptureFPS
	if captureFPS <= 0 {
		captureFPS = 30
	}
	values[SettingKeyCaptureFPS] = strconv.Itoa(captureFPS)

	values[SettingKeyTLSSkipVerify] = strconv.FormatBool(cfg.TLSSkipVerify)
	values[SettingKeyTLSCAFile] = strings.TrimSpace(cfg.TLSCAFile)

	values[SettingKeyAllowRemoteOverrides] = strconv.FormatBool(cfg.AllowRemoteOverrides)

	logLevel := strings.TrimSpace(strings.ToLower(cfg.LogLevel))
	if logLevel == "" {
		logLevel = "info"
	}
	values[SettingKeyLogLevel] = logLevel

	return values
}

// DurationSeconds returns the whole number of seconds in d, or 0 for
// non-positive durations. Exported so the parent package can reuse it.
func DurationSeconds(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	return int64(d.Seconds())
}
