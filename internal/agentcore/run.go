package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentcore/backends"
	dockerpkg "github.com/labtether/labtether/internal/agentcore/docker"
	"github.com/labtether/labtether/internal/agentcore/files"
	"github.com/labtether/labtether/internal/agentcore/system"
	"github.com/labtether/labtether/internal/agentmgr"
)

func init() {
	// Wire the files subpackage's desktop session detector to root agentcore's
	// detectDesktopSession so that file path resolution can discover the active
	// desktop session without a circular import.
	files.DetectDesktopSessionFn = func() files.DesktopSessionInfo {
		session := detectDesktopSessionFn()
		return files.DesktopSessionInfo{
			Username: session.Username,
			UID:      session.UID,
		}
	}

	// Clipboard wiring is in init_clipboard_linux.go (build-tagged).
}

// Run wires the shared runtime with heartbeat publishing and starts HTTP endpoints.
// If LABTETHER_WS_URL is configured, it establishes a WebSocket transport to the
// hub and uses it for heartbeats, telemetry, and command execution. The HTTP
// heartbeat publisher is kept as fallback during WebSocket disconnects.
func Run(ctx context.Context, cfg RuntimeConfig, provider TelemetryProvider) error {
	setConnectorDiscoveryDockerConfig(cfg.DockerEnabled, cfg.DockerSocket)

	identity, identityErr := ensureDeviceIdentity(cfg)
	if identityErr != nil {
		log.Printf("%s: device identity initialization failed: %v", cfg.Name, identityErr)
	}

	if cfg.AutoUpdateEnabled {
		if err := maybeAutoUpdateOnStartup(cfg); err != nil {
			log.Printf("%s: auto-update check skipped: %v", cfg.Name, err)
		}
	}

	// Resolve API token: explicit env → persisted file → enrollment
	if err := ResolveToken(ctx, &cfg); err != nil {
		log.Printf("%s: token resolution failed: %v", cfg.Name, err)
	}

	if cfg.TLSSkipVerify {
		log.Printf("%s: WARNING: TLS certificate verification is disabled (LABTETHER_TLS_SKIP_VERIFY=true). This is insecure and should only be used for initial setup. Configure LABTETHER_TLS_CA_FILE to trust the hub CA.", cfg.Name)
	}

	staticMeta := provider.StaticMetadata()
	if staticMeta == nil {
		staticMeta = map[string]string{}
	}
	if version := strings.TrimSpace(cfg.Version); version != "" {
		staticMeta["agent_version"] = version
	}
	webrtcCaps := detectWebRTCCapabilitiesForConfig(cfg)
	capabilities := []string{"terminal", "desktop", "files"}
	if sessionType := strings.TrimSpace(webrtcCaps.DesktopSessionType); sessionType != "" {
		staticMeta["desktop_session_type"] = sessionType
	}
	if backend := strings.TrimSpace(webrtcCaps.DesktopBackend); backend != "" {
		staticMeta["desktop_backend"] = backend
	}
	if captureBackend := strings.TrimSpace(webrtcCaps.CaptureBackend); captureBackend != "" {
		staticMeta["desktop_capture_backend"] = captureBackend
	}
	staticMeta["desktop_vnc_real_desktop_supported"] = fmt.Sprintf("%v", webrtcCaps.VNCRealDesktopSupported)
	staticMeta["desktop_webrtc_real_desktop_supported"] = fmt.Sprintf("%v", webrtcCaps.WebRTCRealDesktopSupported)
	if webrtcCaps.Available {
		capabilities = append(capabilities, "webrtc")
		staticMeta["webrtc_available"] = "true"
		delete(staticMeta, "webrtc_unavailable_reason")
		if len(webrtcCaps.VideoEncoders) > 0 {
			staticMeta["webrtc_video_encoders"] = strings.Join(webrtcCaps.VideoEncoders, ",")
		}
		if len(webrtcCaps.AudioSources) > 0 {
			staticMeta["webrtc_audio_sources"] = strings.Join(webrtcCaps.AudioSources, ",")
		}
	} else {
		staticMeta["webrtc_available"] = "false"
		if reason := strings.TrimSpace(webrtcCaps.UnavailableReason); reason != "" {
			staticMeta["webrtc_unavailable_reason"] = reason
		}
	}
	if identity != nil {
		staticMeta["agent_device_fingerprint"] = identity.Fingerprint
		staticMeta["agent_device_key_alg"] = identity.KeyAlgorithm
	}
	httpPublisher := NewHeartbeatPublisher(cfg, staticMeta)

	var publisher HeartbeatPublisher
	var transport *wsTransport

	if cfg.WSBaseURL != "" {
		platform := ""
		if meta := staticMeta; meta != nil {
			platform = meta["platform"]
		}

		transport = newWSTransport(cfg.WSBaseURL, cfg.APIToken, cfg.AssetID, platform, cfg.Version, buildTLSConfig(&cfg), cfg.TokenFilePath, identity)

		// Set re-enrollment callback if enrollment token is configured.
		if cfg.EnrollmentToken != "" {
			transport.reEnrollFn = func() (string, error) {
				cfgCopy := cfg
				cfgCopy.APIToken = "" // force re-enrollment path
				if err := ResolveToken(ctx, &cfgCopy); err != nil {
					return "", err
				}
				if cfgCopy.APIToken == "" {
					return "", fmt.Errorf("re-enrollment returned empty token")
				}
				// Persist the new token to disk for next startup.
				if cfg.TokenFilePath != "" {
					_ = saveTokenToFile(cfg.TokenFilePath, cfgCopy.APIToken)
				}
				// Note: transport.updateToken() is called by the reconnect loop
				// after this returns; no need to mutate the outer cfg.
				return cfgCopy.APIToken, nil
			}
		}

		// Start network change monitor.
		networkCh := make(chan struct{}, 1)
		transport.networkChanged = networkCh
		netMon := newNetworkMonitor(networkCh)
		go netMon.Run(ctx)

		// Buffer for telemetry samples during disconnection.
		telemetryBuf := NewRingBuffer[TelemetrySample](60)

		publisher = newWSHeartbeatPublisher(transport, httpPublisher, cfg, staticMeta, capabilities)

		// Store buffer reference in runtime for telemetry buffering during disconnect.
		runtime := NewRuntime(cfg, provider, publisher)
		runtime.transport = transport
		runtime.telemetryBuf = telemetryBuf
		runtime.deviceIdentity = identity

		// Docker and web service collectors: declared here (before reconnect loop)
		// so the onConnect closure can reference them for state resets.
		var dockerCollector *dockerpkg.DockerCollector
		var execMgr *dockerpkg.DockerExecManager
		var dockerLogMgr *dockerpkg.DockerLogManager
		var webServiceCollector *WebServiceCollector

		// Session managers: created before reconnect loop so onConnect can
		// close stale sessions from the previous connection.
		termMgr := newTerminalManager()
		dispMgr := newDisplayManager()
		deskMgr := newDesktopManager(dispMgr)
		fileMgr := files.NewManager(cfg.FileRootMode)
		webrtcMgr := newWebRTCManager(webrtcCaps, runtime, fileMgr, dispMgr)

		// Start reconnect loop in background.
		go transport.reconnectLoop(ctx, func() {
			replayBufferedTelemetry(transport, telemetryBuf)
			sendWebRTCCapabilities(transport, webrtcCaps)
			sendAgentSettingsState(transport, runtime, time.Now().UTC().Format(time.RFC3339Nano))
			if dockerCollector != nil {
				dockerCollector.ResetPublishedState()
			}
			if webServiceCollector != nil {
				webServiceCollector.ResetPublishedState()
			}
			// Close stale sessions from the previous connection — the hub
			// has no knowledge of them after reconnect.
			termMgr.CloseAll()
			deskMgr.CloseAll()
			webrtcMgr.CloseAll()
		})

		// Load persisted config overrides from disk.
		loadPersistedConfig(runtime)
		processMgr := system.NewProcessManager()
		serviceMgr := backends.NewServiceManager()
		journalMgr := backends.NewJournalManager()
		diskMgr := system.NewDiskManager()
		networkMgr := newNetworkManager()
		packageMgr := backends.NewPackageManager()
		cronMgr := backends.NewCronManager()
		usersMgr := system.NewUsersManager()
		clipMgr := newClipboardManager()
		audioMgr := newAudioSidebandManager()
		dockerMode := strings.TrimSpace(strings.ToLower(cfg.DockerEnabled))
		if dockerMode == "" {
			dockerMode = "auto"
		}
		if dockerMode == "false" {
			log.Printf("%s: Docker collector disabled by configuration", cfg.Name)
		} else {
			dockerCollector = dockerpkg.NewDockerCollector(cfg.DockerSocket, transport, cfg.AssetID, cfg.DockerDiscoveryInterval)
			if dockerCollector.IsAvailable() {
				execMgr = dockerCollector.NewExecManager()
				dockerLogMgr = dockerCollector.NewLogManager()
				go dockerCollector.Run(ctx)
				log.Printf("%s: Docker collector enabled (%s): %s", cfg.Name, dockerMode, cfg.DockerSocket)
			} else {
				dockerCollector = nil
				if dockerMode == "true" {
					log.Printf("%s: Docker collector explicitly enabled but endpoint unavailable: %s", cfg.Name, cfg.DockerSocket)
				} else {
					log.Printf("%s: Docker endpoint not available at %s, Docker collector disabled", cfg.Name, cfg.DockerSocket)
				}
			}
		}

		// Web service collector: discovers web services from Docker containers.
		hostIP := resolveHostIP()
		webServiceCollector = NewWebServiceCollector(transport, cfg.AssetID, hostIP, cfg.WebServiceDiscoveryInterval, dockerCollector, WebServiceDiscoveryConfig{
			DockerEnabled:            cfg.ServicesDiscoveryDockerEnabled,
			ProxyEnabled:             cfg.ServicesDiscoveryProxyEnabled,
			ProxyTraefikEnabled:      cfg.ServicesDiscoveryProxyTraefikEnabled,
			ProxyCaddyEnabled:        cfg.ServicesDiscoveryProxyCaddyEnabled,
			ProxyNPMEnabled:          cfg.ServicesDiscoveryProxyNPMEnabled,
			PortScanEnabled:          cfg.ServicesDiscoveryPortScanEnabled,
			PortScanIncludeListening: cfg.ServicesDiscoveryPortScanIncludeListening,
			PortScanPorts:            cfg.ServicesDiscoveryPortScanPorts,
			LANScanEnabled:           cfg.ServicesDiscoveryLANScanEnabled,
			LANScanCIDRs:             cfg.ServicesDiscoveryLANScanCIDRs,
			LANScanPorts:             cfg.ServicesDiscoveryLANScanPorts,
			LANScanMaxHosts:          cfg.ServicesDiscoveryLANScanMaxHosts,
		})
		go webServiceCollector.Run(ctx)
		log.Printf("%s: Web service collector enabled (host IP: %s)", cfg.Name, hostIP)

		go receiveLoop(ctx, transport, cfg, runtime, termMgr, deskMgr, webrtcMgr, fileMgr, processMgr, serviceMgr, journalMgr, diskMgr, networkMgr, packageMgr, cronMgr, usersMgr, clipMgr, audioMgr, dockerCollector, webServiceCollector, execMgr, dockerLogMgr)

		// Start log producer — runs independently of receiveLoop, pushing
		// journalctl entries to the hub as MsgLogBatch messages.
		if cfg.LogStreamEnabled {
			logMgr := backends.NewLogManager()
			defer logMgr.CloseAll()
			go logMgr.Start(ctx, transport)
		} else {
			log.Printf("%s: background log streaming disabled (LABTETHER_LOG_STREAM_ENABLED=false)", cfg.Name)
		}

		log.Printf("%s: WebSocket transport configured: %s", cfg.Name, cfg.WSBaseURL)

		return runtime.Run(ctx)
	}

	publisher = httpPublisher
	runtime := NewRuntime(cfg, provider, publisher)
	runtime.deviceIdentity = identity
	return runtime.Run(ctx)
}

func replayBufferedTelemetry(transport *wsTransport, telemetryBuf *RingBuffer[TelemetrySample]) {
	if transport == nil || telemetryBuf == nil {
		return
	}

	buffered := telemetryBuf.Drain()
	if len(buffered) == 0 {
		return
	}

	log.Printf("agentws: replaying %d buffered telemetry samples", len(buffered))
	for _, sample := range buffered {
		sendTelemetrySample(transport, sample)
	}
}

func sendWebRTCCapabilities(transport *wsTransport, caps agentmgr.WebRTCCapabilitiesData) {
	if transport == nil || !transport.Connected() {
		return
	}
	data, err := json.Marshal(caps)
	if err != nil {
		return
	}
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgWebRTCCapabilities,
		Data: data,
	})
}
