package agentcore

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
)

type Runtime struct {
	cfg       RuntimeConfig
	provider  TelemetryProvider
	publisher HeartbeatPublisher

	// WebSocket transport (nil when using HTTP-only mode).
	transport      *wsTransport
	telemetryBuf   *RingBuffer[TelemetrySample]
	deviceIdentity *deviceIdentity

	// Dynamic config overrides (0 = use default from cfg).
	collectIntervalOverride   atomic.Int64
	heartbeatIntervalOverride atomic.Int64
	baseCollectInterval       time.Duration
	baseHeartbeatInterval     time.Duration

	startedAt time.Time
	alerts    []AlertSnapshot

	mu               sync.RWMutex
	sample           TelemetrySample
	lastCollectErr   string
	lastCollectErrAt time.Time
	lastPublishErr   string
	lastPublishErrAt time.Time
	localBindAddress string
	localAuthEnabled bool
}

func NewRuntime(cfg RuntimeConfig, provider TelemetryProvider, publisher HeartbeatPublisher) *Runtime {
	now := time.Now().UTC()
	return &Runtime{
		cfg:                   cfg,
		provider:              provider,
		publisher:             publisher,
		baseCollectInterval:   cfg.CollectInterval,
		baseHeartbeatInterval: cfg.HeartbeatInterval,
		startedAt:             now,
		sample: TelemetrySample{
			AssetID:     cfg.AssetID,
			CollectedAt: now,
		},
	}
}

func (r *Runtime) Run(ctx context.Context) error {
	bindAddress := resolveAgentLocalBindAddress()
	localAuthToken, err := resolveAgentLocalAuthToken(r.cfg, bindAddress)
	if err != nil {
		return err
	}
	r.localBindAddress = bindAddress
	r.localAuthEnabled = strings.TrimSpace(localAuthToken) != ""

	go r.collectLoop(ctx)
	go r.heartbeatLoop(ctx)

	return servicehttp.Run(ctx, servicehttp.Config{
		Name:        r.cfg.Name,
		Port:        r.cfg.Port,
		BindAddress: bindAddress,
		AuthToken:   localAuthToken,
		ExtraHandlers: map[string]http.HandlerFunc{
			"/agent/info": func(w http.ResponseWriter, req *http.Request) {
				servicehttp.WriteJSON(w, http.StatusOK, r.provider.AgentInfo())
			},
			"/agent/telemetry": func(w http.ResponseWriter, req *http.Request) {
				servicehttp.WriteJSON(w, http.StatusOK, r.current())
			},
			"/agent/status": r.statusHandler(),
			"/metrics": func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
				_, _ = w.Write([]byte(RenderPrometheus(r.current())))
			},
		},
	})
}

const (
	envAgentLocalBindAddress       = "LABTETHER_AGENT_LOCAL_BIND_ADDRESS"
	envAgentLocalAuthToken         = "LABTETHER_AGENT_LOCAL_AUTH_TOKEN"      // #nosec G101 -- environment variable key name, not an embedded credential.
	envAgentLocalAuthTokenFile     = "LABTETHER_AGENT_LOCAL_AUTH_TOKEN_FILE" // #nosec G101 -- environment variable key name, not an embedded credential.
	envAgentLocalAllowUnauth       = "LABTETHER_AGENT_LOCAL_ALLOW_UNAUTHENTICATED"
	defaultAgentLocalBindAddress   = "127.0.0.1"
	defaultAgentLocalBindAddressV6 = "::1"
)

func resolveAgentLocalBindAddress() string {
	bindAddress := strings.TrimSpace(os.Getenv(envAgentLocalBindAddress))
	if bindAddress == "" {
		return defaultAgentLocalBindAddress
	}
	return bindAddress
}

func resolveAgentLocalAuthToken(cfg RuntimeConfig, bindAddress string) (string, error) {
	if token := strings.TrimSpace(os.Getenv(envAgentLocalAuthToken)); token != "" {
		return token, nil
	}
	if tokenPath := strings.TrimSpace(os.Getenv(envAgentLocalAuthTokenFile)); tokenPath != "" {
		token, err := loadSecretFromFile(tokenPath)
		if err != nil {
			return "", fmt.Errorf("failed to load %s: %w", envAgentLocalAuthTokenFile, err)
		}
		if token = strings.TrimSpace(token); token != "" {
			return token, nil
		}
	}
	if isLoopbackBindAddress(bindAddress) {
		return "", nil
	}
	if parseBoolEnv(envAgentLocalAllowUnauth, false) {
		log.Printf("%s: WARNING: non-loopback local API binding is unauthenticated (%s=true)", cfg.Name, envAgentLocalAllowUnauth)
		return "", nil
	}
	if token := strings.TrimSpace(cfg.APIToken); token != "" {
		return token, nil
	}
	return "", fmt.Errorf("non-loopback local API binding requires %s or %s=true", envAgentLocalAuthToken, envAgentLocalAllowUnauth)
}

func isLoopbackBindAddress(value string) bool {
	normalized := strings.TrimSpace(strings.ToLower(value))
	switch normalized {
	case "", "localhost", defaultAgentLocalBindAddress, defaultAgentLocalBindAddressV6:
		return true
	}
	if ip := net.ParseIP(normalized); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func (r *Runtime) collectLoop(ctx context.Context) {
	r.collectOnce(time.Now().UTC())
	currentInterval := r.cfg.CollectInterval
	ticker := time.NewTicker(currentInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("%s telemetry collector stopped", r.cfg.Name)
			return
		case tick := <-ticker.C:
			r.collectOnce(tick.UTC())
			// Check for dynamic interval override.
			if override := r.collectIntervalOverride.Load(); override > 0 {
				newInterval := time.Duration(override) * time.Second
				if newInterval != currentInterval {
					currentInterval = newInterval
					ticker.Reset(currentInterval)
					log.Printf("%s: collect interval updated to %v", r.cfg.Name, currentInterval)
				}
			}
		}
	}
}

func (r *Runtime) heartbeatLoop(ctx context.Context) {
	r.publishOnce(ctx)
	currentInterval := r.cfg.HeartbeatInterval
	ticker := time.NewTicker(currentInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("%s heartbeat loop stopped", r.cfg.Name)
			return
		case <-ticker.C:
			r.publishOnce(ctx)
			// Check for dynamic interval override.
			if override := r.heartbeatIntervalOverride.Load(); override > 0 {
				newInterval := time.Duration(override) * time.Second
				if newInterval != currentInterval {
					currentInterval = newInterval
					ticker.Reset(currentInterval)
					log.Printf("%s: heartbeat interval updated to %v", r.cfg.Name, currentInterval)
				}
			}
		}
	}
}

func (r *Runtime) collectOnce(now time.Time) {
	sample, err := r.provider.Collect(now)
	if err != nil {
		r.logCollectWarning(err)
	}
	if sample.AssetID == "" {
		sample.AssetID = r.cfg.AssetID
	}
	if sample.CollectedAt.IsZero() {
		sample.CollectedAt = now
	}
	sample.CPUPercent = ClampPercent(sample.CPUPercent)
	sample.MemoryPercent = ClampPercent(sample.MemoryPercent)
	sample.DiskPercent = ClampPercent(sample.DiskPercent)

	r.mu.Lock()
	r.sample = sample
	r.mu.Unlock()
}

func (r *Runtime) publishOnce(ctx context.Context) {
	sample := r.current()
	if err := r.publisher.Publish(ctx, sample); err != nil {
		r.logPublishWarning(err)
	}

	// Also send a dedicated telemetry message over WebSocket.
	if r.transport != nil {
		if r.transport.Connected() {
			sendTelemetrySample(r.transport, sample)
		} else if r.telemetryBuf != nil {
			r.telemetryBuf.Push(sample)
		}
	}
}

func (r *Runtime) current() TelemetrySample {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sample
}

func (r *Runtime) logCollectWarning(err error) {
	r.logWarning("collect", err, &r.lastCollectErr, &r.lastCollectErrAt)
}

func (r *Runtime) logPublishWarning(err error) {
	r.logWarning("heartbeat", err, &r.lastPublishErr, &r.lastPublishErrAt)
}

func (r *Runtime) logWarning(scope string, err error, lastMessage *string, lastAt *time.Time) {
	if err == nil {
		return
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "unknown error"
	}
	now := time.Now().UTC()

	r.mu.Lock()
	if message == *lastMessage && now.Sub(*lastAt) < 5*time.Minute {
		r.mu.Unlock()
		return
	}
	*lastMessage = message
	*lastAt = now
	r.mu.Unlock()

	log.Printf("%s %s warning: %s", r.cfg.Name, scope, message)
}
