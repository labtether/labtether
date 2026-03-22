package agentcore

// Backward-compatible type aliases and function wrappers for types moved to
// the sysconfig subpackage. This allows the rest of the agentcore root package
// to continue using the same identifiers without import changes.

import (
	"time"

	"github.com/labtether/labtether/internal/agentcore/sysconfig"
	webservicepkg "github.com/labtether/labtether/internal/agentcore/webservice"
)

// Setting type aliases.
type AgentSettingType = sysconfig.AgentSettingType

const (
	AgentSettingTypeString = sysconfig.AgentSettingTypeString
	AgentSettingTypeInt    = sysconfig.AgentSettingTypeInt
	AgentSettingTypeBool   = sysconfig.AgentSettingTypeBool
	AgentSettingTypeEnum   = sysconfig.AgentSettingTypeEnum
)

// Setting key constants.
const (
	SettingKeyCollectIntervalSec                        = sysconfig.SettingKeyCollectIntervalSec
	SettingKeyHeartbeatIntervalSec                      = sysconfig.SettingKeyHeartbeatIntervalSec
	SettingKeyDockerEnabled                             = sysconfig.SettingKeyDockerEnabled
	SettingKeyDockerEndpoint                            = sysconfig.SettingKeyDockerEndpoint
	SettingKeyDockerDiscoveryIntervalSec                = sysconfig.SettingKeyDockerDiscoveryIntervalSec
	SettingKeyServicesDiscoveryDockerEnabled            = sysconfig.SettingKeyServicesDiscoveryDockerEnabled
	SettingKeyServicesDiscoveryProxyEnabled             = sysconfig.SettingKeyServicesDiscoveryProxyEnabled
	SettingKeyServicesDiscoveryProxyTraefikEnabled      = sysconfig.SettingKeyServicesDiscoveryProxyTraefikEnabled
	SettingKeyServicesDiscoveryProxyCaddyEnabled        = sysconfig.SettingKeyServicesDiscoveryProxyCaddyEnabled
	SettingKeyServicesDiscoveryProxyNPMEnabled          = sysconfig.SettingKeyServicesDiscoveryProxyNPMEnabled
	SettingKeyServicesDiscoveryPortScanEnabled          = sysconfig.SettingKeyServicesDiscoveryPortScanEnabled
	SettingKeyServicesDiscoveryPortScanIncludeListening = sysconfig.SettingKeyServicesDiscoveryPortScanIncludeListening
	SettingKeyServicesDiscoveryPortScanPorts            = sysconfig.SettingKeyServicesDiscoveryPortScanPorts
	SettingKeyServicesDiscoveryLANScanEnabled           = sysconfig.SettingKeyServicesDiscoveryLANScanEnabled
	SettingKeyServicesDiscoveryLANScanCIDRs             = sysconfig.SettingKeyServicesDiscoveryLANScanCIDRs
	SettingKeyServicesDiscoveryLANScanPorts             = sysconfig.SettingKeyServicesDiscoveryLANScanPorts
	SettingKeyServicesDiscoveryLANScanMaxHosts          = sysconfig.SettingKeyServicesDiscoveryLANScanMaxHosts
	SettingKeyFilesRootMode                             = sysconfig.SettingKeyFilesRootMode
	SettingKeyWebRTCEnabled                             = sysconfig.SettingKeyWebRTCEnabled
	SettingKeyWebRTCSTUNURL                             = sysconfig.SettingKeyWebRTCSTUNURL
	SettingKeyWebRTCTURNURL                             = sysconfig.SettingKeyWebRTCTURNURL
	SettingKeyWebRTCTURNUser                            = sysconfig.SettingKeyWebRTCTURNUser
	SettingKeyWebRTCTURNPass                            = sysconfig.SettingKeyWebRTCTURNPass
	SettingKeyWebRTCWaylandExperimentalEnabled          = sysconfig.SettingKeyWebRTCWaylandExperimentalEnabled
	SettingKeyWebRTCWaylandPipeWireNodeID               = sysconfig.SettingKeyWebRTCWaylandPipeWireNodeID
	SettingKeyWebRTCWaylandInputBackend                 = sysconfig.SettingKeyWebRTCWaylandInputBackend
	SettingKeyCaptureFPS                                = sysconfig.SettingKeyCaptureFPS
	SettingKeyAllowRemoteOverrides                      = sysconfig.SettingKeyAllowRemoteOverrides
	SettingKeyLogLevel                                  = sysconfig.SettingKeyLogLevel
	SettingKeyTLSSkipVerify                             = sysconfig.SettingKeyTLSSkipVerify
	SettingKeyTLSCAFile                                 = sysconfig.SettingKeyTLSCAFile
)

// Setting definition type alias.
type AgentSettingDefinition = sysconfig.AgentSettingDefinition

// Setting definition functions.
var AgentSettingDefinitions = sysconfig.AgentSettingDefinitions
var AgentSettingDefinitionByKey = sysconfig.AgentSettingDefinitionByKey
var NormalizeAgentSettingValue = sysconfig.NormalizeAgentSettingValue
var DefaultAgentSettingValues = sysconfig.DefaultAgentSettingValues
var normalizeDiscoveryPortListValue = sysconfig.NormalizeDiscoveryPortListValue
var normalizeDiscoveryCIDRListValue = sysconfig.NormalizeDiscoveryCIDRListValue

// Settings file functions.
var LoadAgentSettingsFile = sysconfig.LoadAgentSettingsFile
var SaveAgentSettingsFile = sysconfig.SaveAgentSettingsFile

// Settings values — ConfigSnapshot alias and function.
type ConfigSnapshot = sysconfig.ConfigSnapshot

var AgentSettingValuesFromConfigSnapshot = sysconfig.AgentSettingValuesFromConfig

// durationSeconds is kept as a direct import for internal use in root.
// It delegates to sysconfig.DurationSeconds.
func durationSeconds(d time.Duration) int64 { return sysconfig.DurationSeconds(d) }

// Network manager type alias.
type networkManager = sysconfig.NetworkManager

var newNetworkManager = sysconfig.NewNetworkManager
var collectNetworkInterfaces = sysconfig.CollectNetworkInterfaces

// Network command helpers.
var hasCommand = sysconfig.HasCommand
var runCommandWithTimeout = sysconfig.RunCommandWithTimeout
var truncateCommandOutput = sysconfig.TruncateCommandOutput

const maxCommandOutputBytes = sysconfig.MaxCommandOutputBytes

// Network method resolver and functions — test code should reference
// sysconfig.ResolveNetworkMethodFn etc. directly since they are package-level
// var function hooks in sysconfig.

// Network backend types.
type networkBackend = sysconfig.NetworkBackend
type linuxNetworkBackend = sysconfig.LinuxNetworkBackend
type darwinNetworkBackend = sysconfig.DarwinNetworkBackend
type unsupportedNetworkBackend = sysconfig.UnsupportedNetworkBackend
type darwinNetworkSnapshot = sysconfig.DarwinNetworkSnapshot
type darwinNetworkService = sysconfig.DarwinNetworkService

var newNetworkBackend = sysconfig.NewNetworkBackend
var parseDarwinNetworkServicesOutput = sysconfig.ParseDarwinNetworkServicesOutput
var parseDarwinDNSServersOutput = sysconfig.ParseDarwinDNSServersOutput
var parseDarwinNetworkServiceEnabledOutput = sysconfig.ParseDarwinNetworkServiceEnabledOutput
var resolveDarwinNetworkMethodWith = sysconfig.ResolveDarwinNetworkMethodWith
var cloneStringSlice = sysconfig.CloneStringSlice
var cloneDarwinNetworkSnapshot = sysconfig.CloneDarwinNetworkSnapshot
var resolveNetworkMethod = sysconfig.ResolveNetworkMethod
var verifyConnectivity = sysconfig.VerifyConnectivity
var parseDefaultRouteGateway = sysconfig.ParseDefaultRouteGateway

// Network monitor type alias.
type networkMonitor = sysconfig.NetworkMonitor

var newNetworkMonitor = sysconfig.NewNetworkMonitor
var getLocalIPs = sysconfig.GetLocalIPs

// Display functions.
var platformListDisplaysFn = sysconfig.PlatformListDisplaysFn
var handleListDisplays = sysconfig.HandleListDisplays
var parseXrandrMonitors = sysconfig.ParseXrandrMonitors
var parseResolution = sysconfig.ParseResolution
var parsePowerShellScreenDisplays = sysconfig.ParsePowerShellScreenDisplays

// Clipboard type alias.
type clipboardManager = sysconfig.ClipboardManager

var newClipboardManager = sysconfig.NewClipboardManager
var clipboardRead = sysconfig.ClipboardRead
var clipboardWriteText = sysconfig.ClipboardWriteText
var clipboardWriteImage = sysconfig.ClipboardWriteImage

// Platform clipboard functions — direct access for use by webrtc_handler.
var platformClipboardRead = sysconfig.PlatformClipboardRead
var platformClipboardWriteText = sysconfig.PlatformClipboardWriteText
var platformClipboardWriteImage = sysconfig.PlatformClipboardWriteImage

// readIfaceStats aliases the platform-specific function.
var readIfaceStats = sysconfig.ReadIfaceStats

// WebService subpackage type aliases.
type WebServiceCollector = webservicepkg.WebServiceCollector
type WebServiceDiscoveryConfig = webservicepkg.WebServiceDiscoveryConfig

var NewWebServiceCollector = webservicepkg.NewWebServiceCollector
var resolveHostIP = webservicepkg.ResolveHostIP

// WebService registry type aliases for external callers.
type KnownService = webservicepkg.KnownService

const CatOther = webservicepkg.CatOther

var LookupByDockerImage = webservicepkg.LookupByDockerImage
var LookupByHint = webservicepkg.LookupByHint
