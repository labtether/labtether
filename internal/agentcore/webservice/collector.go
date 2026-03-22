package webservice

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	dockerpkg "github.com/labtether/labtether/internal/agentcore/docker"
	proxypkg "github.com/labtether/labtether/internal/agentcore/proxy"
	"github.com/labtether/labtether/internal/agentcore/sysconfig"
	"github.com/labtether/labtether/internal/agentmgr"
)

const (
	defaultWebServiceInterval = 60 * time.Second
	healthCheckTimeout        = 2 * time.Second
	maxUnknownServicePorts    = 4
	portScanDialTimeout       = 250 * time.Millisecond
	maxPortScanConcurrency    = 16
	maxLANScanConcurrency     = 64
	maxHealthCheckConcurrency = 16
	maxUnknownScannedServices = 8
	maxUnknownLANScanned      = 16
	maxListeningScanPorts     = 128
	maxLANScanPorts           = 64
	maxLANScanCIDRs           = 16
	maxLANScanHosts           = 1024

	// Cache caps avoid long-lived growth when service endpoints churn over time.
	maxCompatCacheEntries      = 1024
	maxFingerprintCacheEntries = 1024
	maxHealthCacheEntries      = 2048
)

var defaultPortScanCandidates = []int{
	80, 81, 82, 88, 443, 591, 593, 631,
	2375, 2376,
	3000, 3001, 3002, 4000, 5000, 5055, 5601,
	6001, 7000, 7080, 7081, 7443, 7474,
	8000, 8001, 8006, 8007, 8080, 8081, 8082, 8088, 8089, 8090, 8091, 8096, 8123, 8181, 8443,
	8888, 8989, 9000, 9001, 9090, 9091, 9443, 9696, 10443, 19999, 32400,
}

type WebServiceDiscoveryConfig struct {
	DockerEnabled            bool
	ProxyEnabled             bool
	ProxyTraefikEnabled      bool
	ProxyCaddyEnabled        bool
	ProxyNPMEnabled          bool
	PortScanEnabled          bool
	PortScanIncludeListening bool
	PortScanPorts            string
	LANScanEnabled           bool
	LANScanCIDRs             string
	LANScanPorts             string
	LANScanMaxHosts          int
}

// WebServiceCollector discovers web services from Docker containers and health-checks them.
// It follows the same Run() pattern as DockerCollector.
type WebServiceCollector struct {
	mu               sync.Mutex
	transport        Transport
	assetID          string
	hostIP           string
	interval         time.Duration
	docker           *dockerpkg.DockerCollector // optional, reuse Docker API access
	client           *http.Client               // for health checks
	insecureClient   *http.Client               // reserved for explicit secure test injection
	discoveryCfg     WebServiceDiscoveryConfig
	proxyProviders   []proxypkg.Provider
	lastServices     []agentmgr.DiscoveredWebService
	compatCache      map[string]compatCacheEntry
	fingerprintCache map[string]fingerprintCacheEntry
	healthCache      map[string]healthCacheEntry
	nowFn            func() time.Time
}

type healthCacheEntry struct {
	inputURL   string
	outputURL  string
	status     string
	responseMs int
	checkedAt  time.Time
}

// NewWebServiceCollector creates a collector that discovers web services from Docker containers.
// If docker is nil, Docker-based discovery is skipped. hostIP is the IP to use in service URLs;
// if empty, it will be resolved automatically on first discovery cycle.
func NewWebServiceCollector(transport Transport, assetID string, hostIP string, interval time.Duration, docker *dockerpkg.DockerCollector, cfg WebServiceDiscoveryConfig) *WebServiceCollector {
	if interval <= 0 {
		interval = defaultWebServiceInterval
	}
	cfg = normalizeWebServiceDiscoveryConfig(cfg)

	var providers []proxypkg.Provider
	if cfg.ProxyEnabled {
		if cfg.ProxyTraefikEnabled {
			providers = append(providers, proxypkg.NewTraefikProvider())
		}
		if cfg.ProxyCaddyEnabled {
			providers = append(providers, proxypkg.NewCaddyProvider())
		}
		if cfg.ProxyNPMEnabled {
			providers = append(providers, proxypkg.NewNPMProvider())
		}
	}

	redirectPolicy := func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse // don't follow redirects
	}
	secureTransport := http.DefaultTransport.(*http.Transport).Clone()
	insecureTransport := http.DefaultTransport.(*http.Transport).Clone()
	if insecureTransport.TLSClientConfig == nil {
		insecureTransport.TLSClientConfig = &tls.Config{} // #nosec G402 -- discovery probe client only.
	} else {
		insecureTransport.TLSClientConfig = insecureTransport.TLSClientConfig.Clone()
	}
	insecureTransport.TLSClientConfig.InsecureSkipVerify = true // #nosec G402 -- discovery probe client only.

	return &WebServiceCollector{
		transport:      transport,
		assetID:        assetID,
		hostIP:         hostIP,
		interval:       interval,
		docker:         docker,
		discoveryCfg:   cfg,
		proxyProviders: providers,
		client: &http.Client{
			Timeout:       healthCheckTimeout,
			Transport:     secureTransport,
			CheckRedirect: redirectPolicy,
		},
		insecureClient: &http.Client{
			Timeout:       healthCheckTimeout,
			Transport:     insecureTransport,
			CheckRedirect: redirectPolicy,
		},
		compatCache:      make(map[string]compatCacheEntry),
		fingerprintCache: make(map[string]fingerprintCacheEntry),
		healthCache:      make(map[string]healthCacheEntry),
		nowFn:            func() time.Time { return time.Now().UTC() },
	}
}

func normalizeWebServiceDiscoveryConfig(cfg WebServiceDiscoveryConfig) WebServiceDiscoveryConfig {
	if cfg == (WebServiceDiscoveryConfig{}) {
		cfg = defaultWebServiceDiscoveryConfigFromEnv()
	}

	if cfg.LANScanMaxHosts <= 0 {
		cfg.LANScanMaxHosts = 64
	}
	if cfg.LANScanMaxHosts > maxLANScanHosts {
		cfg.LANScanMaxHosts = maxLANScanHosts
	}

	if normalized, err := sysconfig.NormalizeDiscoveryPortListValue(sysconfig.SettingKeyServicesDiscoveryPortScanPorts, cfg.PortScanPorts); err == nil {
		cfg.PortScanPorts = normalized
	} else {
		cfg.PortScanPorts = ""
	}
	if normalized, err := sysconfig.NormalizeDiscoveryPortListValue(sysconfig.SettingKeyServicesDiscoveryLANScanPorts, cfg.LANScanPorts); err == nil {
		cfg.LANScanPorts = normalized
	} else {
		cfg.LANScanPorts = ""
	}
	if normalized, err := sysconfig.NormalizeDiscoveryCIDRListValue(sysconfig.SettingKeyServicesDiscoveryLANScanCIDRs, cfg.LANScanCIDRs); err == nil {
		cfg.LANScanCIDRs = normalized
	} else {
		cfg.LANScanCIDRs = ""
	}

	return cfg
}

func defaultWebServiceDiscoveryConfigFromEnv() WebServiceDiscoveryConfig {
	proxyEnabled := !strings.EqualFold(strings.TrimSpace(os.Getenv("LABTETHER_PROXY_DISABLED")), "true")
	portScanEnabled := !strings.EqualFold(strings.TrimSpace(os.Getenv("LABTETHER_WEBSVC_PORTSCAN_DISABLED")), "true")
	includeListening := !strings.EqualFold(strings.TrimSpace(os.Getenv("LABTETHER_WEBSVC_PORTSCAN_INCLUDE_LISTENING")), "false")

	return WebServiceDiscoveryConfig{
		DockerEnabled:            true,
		ProxyEnabled:             proxyEnabled,
		ProxyTraefikEnabled:      true,
		ProxyCaddyEnabled:        true,
		ProxyNPMEnabled:          true,
		PortScanEnabled:          portScanEnabled,
		PortScanIncludeListening: includeListening,
		PortScanPorts:            strings.TrimSpace(os.Getenv("LABTETHER_WEBSVC_PORTSCAN_PORTS")),
		LANScanEnabled:           false,
		LANScanMaxHosts:          64,
	}
}

// Run starts the periodic web service discovery loop. Blocks until ctx is cancelled.
func (wsc *WebServiceCollector) Run(ctx context.Context) {
	// Resolve host IP if not set
	if wsc.hostIP == "" {
		wsc.hostIP = ResolveHostIP()
	}

	// Initial discovery immediately
	wsc.RunCycle(ctx)

	ticker := time.NewTicker(wsc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			wsc.RunCycle(ctx)
		}
	}
}

func (wsc *WebServiceCollector) RunCycle(ctx context.Context) {
	wsc.mu.Lock()
	defer wsc.mu.Unlock()

	if wsc.transport == nil || !wsc.transport.Connected() {
		return
	}

	cycleStartedAt := time.Now()
	cfg := wsc.discoveryCfg
	now := wsc.now()

	var services []agentmgr.DiscoveredWebService
	var containers []dockerpkg.DockerContainer
	var previous []agentmgr.DiscoveredWebService
	clonePrevious := func() []agentmgr.DiscoveredWebService {
		if previous == nil {
			previous = cloneDiscoveredServices(wsc.lastServices)
		}
		return previous
	}
	var dockerListErr error
	sourceStats := map[string]agentmgr.WebServiceDiscoverySourceStat{
		"docker": {
			Enabled: cfg.DockerEnabled,
		},
		"proxy": {
			Enabled: cfg.ProxyEnabled,
		},
		"local_scan": {
			Enabled: cfg.PortScanEnabled,
		},
		"lan_scan": {
			Enabled: cfg.LANScanEnabled,
		},
	}

	// Docker-based discovery
	if cfg.DockerEnabled && wsc.docker != nil && wsc.docker.HasClient() {
		dockerStartedAt := time.Now()
		containers, dockerListErr = wsc.docker.ListContainers(ctx)
		if dockerListErr != nil {
			log.Printf("webservices: docker list failed: %v", dockerListErr)
		} else {
			dockerServices := wsc.buildServicesFromContainers(containers)
			services = append(services, dockerServices...)
			sourceStats["docker"] = agentmgr.WebServiceDiscoverySourceStat{
				Enabled:       cfg.DockerEnabled,
				DurationMs:    millisecondsSince(dockerStartedAt),
				ServicesFound: len(dockerServices),
			}
		}
		if dockerListErr != nil {
			sourceStats["docker"] = agentmgr.WebServiceDiscoverySourceStat{
				Enabled:       cfg.DockerEnabled,
				DurationMs:    millisecondsSince(dockerStartedAt),
				ServicesFound: 0,
			}
		}
	}

	// Preserve the last known docker services when Docker discovery is temporarily unavailable.
	if cfg.DockerEnabled && dockerListErr != nil {
		services = append(services, filterServicesBySource(clonePrevious(), "docker")...)
	}

	// Proxy enrichment phase — runs even when Docker is down so proxy-only
	// services (discovered purely from reverse proxy APIs) are still found.
	proxyStartedAt := time.Now()
	proxyServicesFound := 0
	if cfg.ProxyEnabled && len(wsc.proxyProviders) > 0 {
		var proxyErr bool
		beforeProxyLen := len(services)
		services, proxyErr = wsc.enrichFromProxies(containers, services)
		if len(services) > beforeProxyLen {
			proxyServicesFound = len(services) - beforeProxyLen
		}
		// Preserve previous proxy-only services during transient provider failures.
		if proxyErr {
			services = append(services, filterServicesBySource(clonePrevious(), "proxy")...)
		}
	}
	sourceStats["proxy"] = agentmgr.WebServiceDiscoverySourceStat{
		Enabled:       cfg.ProxyEnabled,
		DurationMs:    millisecondsSince(proxyStartedAt),
		ServicesFound: proxyServicesFound,
	}

	// Port-scan enrichment phase — discovers host-native services not represented
	// in Docker/proxy inventories yet (for example daemon UIs on bare host ports).
	localScanStartedAt := time.Now()
	localScannedServices := wsc.discoverPortScannedServicesWithConfig(ctx, services, cfg)
	services = append(services, localScannedServices...)
	sourceStats["local_scan"] = agentmgr.WebServiceDiscoverySourceStat{
		Enabled:       cfg.PortScanEnabled,
		DurationMs:    millisecondsSince(localScanStartedAt),
		ServicesFound: len(localScannedServices),
	}

	lanScanStartedAt := time.Now()
	lanScannedServices := wsc.discoverLANScannedServicesWithConfig(ctx, services, cfg)
	services = append(services, lanScannedServices...)
	sourceStats["lan_scan"] = agentmgr.WebServiceDiscoverySourceStat{
		Enabled:       cfg.LANScanEnabled,
		DurationMs:    millisecondsSince(lanScanStartedAt),
		ServicesFound: len(lanScannedServices),
	}

	services = dedupeDiscoveredServices(services)

	// Apply runtime fingerprints for unresolved services, then normalize known
	// multi-surface services, then health check all discovered services (with
	// DNS fallback for proxied URLs).
	for i := range services {
		wsc.applyFingerprintMetadata(&services[i])
	}
	normalizeLabTetherServices(services)
	wsc.applyHealthChecksParallel(services, now)

	// Send report
	discoveryStats := &agentmgr.WebServiceDiscoveryStats{
		CollectedAt:      time.Now().UTC().Format(time.RFC3339),
		CycleDurationMs:  millisecondsSince(cycleStartedAt),
		TotalServices:    len(services),
		Sources:          sourceStats,
		FinalSourceCount: countDiscoveredServicesBySource(services),
	}
	report := agentmgr.WebServiceReportData{
		HostAssetID: wsc.assetID,
		Services:    services,
		Discovery:   discoveryStats,
	}
	data, err := json.Marshal(report)
	if err != nil {
		log.Printf("webservices: failed to marshal report: %v", err)
		return
	}
	if err := wsc.transport.Send(agentmgr.Message{
		Type: agentmgr.MsgWebServiceReport,
		Data: data,
	}); err != nil {
		log.Printf("webservices: failed to send report: %v", err)
		return
	}

	log.Printf(
		"webservices: discovery cycle total=%d duration_ms=%d docker=%d/%dms proxy=%d/%dms local_scan=%d/%dms lan_scan=%d/%dms",
		discoveryStats.TotalServices,
		discoveryStats.CycleDurationMs,
		discoveryStats.Sources["docker"].ServicesFound,
		discoveryStats.Sources["docker"].DurationMs,
		discoveryStats.Sources["proxy"].ServicesFound,
		discoveryStats.Sources["proxy"].DurationMs,
		discoveryStats.Sources["local_scan"].ServicesFound,
		discoveryStats.Sources["local_scan"].DurationMs,
		discoveryStats.Sources["lan_scan"].ServicesFound,
		discoveryStats.Sources["lan_scan"].DurationMs,
	)

	wsc.lastServices = cloneDiscoveredServices(services)
}

// ResetPublishedState clears cached service state so the next cycle sends a
// full report. Called after WebSocket reconnect so the hub gets fresh data.
func (wsc *WebServiceCollector) ResetPublishedState() {
	wsc.mu.Lock()
	wsc.lastServices = nil
	wsc.mu.Unlock()
}

func millisecondsSince(start time.Time) int {
	elapsed := time.Since(start).Milliseconds()
	if elapsed < 0 {
		return 0
	}
	return int(elapsed)
}

func countDiscoveredServicesBySource(services []agentmgr.DiscoveredWebService) map[string]int {
	if len(services) == 0 {
		return map[string]int{}
	}
	out := make(map[string]int, 4)
	for _, svc := range services {
		source := strings.TrimSpace(strings.ToLower(svc.Source))
		if source == "" {
			source = "unknown"
		}
		out[source]++
	}
	return out
}

func (wsc *WebServiceCollector) now() time.Time {
	if wsc != nil && wsc.nowFn != nil {
		return wsc.nowFn().UTC()
	}
	return time.Now().UTC()
}

func healthCacheKeyForService(svc agentmgr.DiscoveredWebService) string {
	if id := strings.TrimSpace(svc.ID); id != "" {
		return "id:" + strings.ToLower(id)
	}
	if base := strings.TrimSpace(serviceFingerprintBaseURL(svc)); base != "" {
		return "url:" + strings.ToLower(base)
	}
	if raw := strings.TrimSpace(svc.URL); raw != "" {
		return "url:" + strings.ToLower(raw)
	}
	return ""
}

func healthCacheInputURL(svc agentmgr.DiscoveredWebService) string {
	if base := strings.TrimSpace(serviceFingerprintBaseURL(svc)); base != "" {
		return base
	}
	return strings.TrimSpace(svc.URL)
}

func (wsc *WebServiceCollector) healthCacheTTL(status string) time.Duration {
	normalized := strings.ToLower(strings.TrimSpace(status))
	if normalized == "down" {
		ttl := wsc.interval
		if ttl < 20*time.Second {
			ttl = 20 * time.Second
		}
		if ttl > 2*time.Minute {
			ttl = 2 * time.Minute
		}
		return ttl
	}
	ttl := wsc.interval * 3
	if ttl < 45*time.Second {
		ttl = 45 * time.Second
	}
	if ttl > 10*time.Minute {
		ttl = 10 * time.Minute
	}
	return ttl
}

// healthCheckJob tracks a service that needs an HTTP health check (cache miss).
type healthCheckJob struct {
	index    int
	cacheKey string
	inputURL string
}

// applyHealthChecksParallel resolves health status for all services using a
// two-pass strategy: cached results are applied sequentially (near-zero cost),
// then cache misses are fanned out across a bounded worker pool so that
// HTTP timeouts overlap instead of stacking.
func (wsc *WebServiceCollector) applyHealthChecksParallel(services []agentmgr.DiscoveredWebService, now time.Time) {
	if wsc.healthCache == nil {
		wsc.healthCache = make(map[string]healthCacheEntry)
	}

	// Pass 1: resolve cache hits sequentially, collect cache misses.
	var uncached []healthCheckJob
	for i := range services {
		svc := &services[i]
		cacheKey := healthCacheKeyForService(*svc)
		inputURL := healthCacheInputURL(*svc)
		if cacheKey != "" {
			if cached, ok := wsc.healthCache[cacheKey]; ok {
				if strings.EqualFold(cached.inputURL, inputURL) && now.Sub(cached.checkedAt) <= wsc.healthCacheTTL(cached.status) {
					svc.Status = cached.status
					svc.ResponseMs = cached.responseMs
					if strings.TrimSpace(cached.outputURL) != "" {
						svc.URL = cached.outputURL
					}
					continue
				}
				delete(wsc.healthCache, cacheKey)
			}
		}
		uncached = append(uncached, healthCheckJob{index: i, cacheKey: cacheKey, inputURL: inputURL})
	}

	if len(uncached) == 0 {
		return
	}

	// Pass 2: health-check cache misses concurrently.
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxHealthCheckConcurrency)
	for _, job := range uncached {
		wg.Add(1)
		sem <- struct{}{}
		go func(j healthCheckJob) {
			defer func() { <-sem; wg.Done() }()
			wsc.healthCheckWithFallback(&services[j.index])
		}(job)
	}
	wg.Wait()

	// Update cache sequentially after all checks complete.
	for _, job := range uncached {
		if job.cacheKey == "" {
			continue
		}
		svc := &services[job.index]
		wsc.healthCache[job.cacheKey] = healthCacheEntry{
			inputURL:   job.inputURL,
			outputURL:  strings.TrimSpace(svc.URL),
			status:     strings.ToLower(strings.TrimSpace(svc.Status)),
			responseMs: svc.ResponseMs,
			checkedAt:  now,
		}
	}
	wsc.pruneHealthCache(now)
}

func (wsc *WebServiceCollector) applyHealthCheckWithCache(svc *agentmgr.DiscoveredWebService, now time.Time) {
	if svc == nil {
		return
	}
	if wsc.healthCache == nil {
		wsc.healthCache = make(map[string]healthCacheEntry)
	}

	cacheKey := healthCacheKeyForService(*svc)
	inputURL := healthCacheInputURL(*svc)
	if cacheKey != "" {
		if cached, ok := wsc.healthCache[cacheKey]; ok {
			if strings.EqualFold(cached.inputURL, inputURL) && now.Sub(cached.checkedAt) <= wsc.healthCacheTTL(cached.status) {
				svc.Status = cached.status
				svc.ResponseMs = cached.responseMs
				if strings.TrimSpace(cached.outputURL) != "" {
					svc.URL = cached.outputURL
				}
				return
			}
			delete(wsc.healthCache, cacheKey)
		}
	}

	wsc.healthCheckWithFallback(svc)
	if cacheKey == "" {
		return
	}
	wsc.healthCache[cacheKey] = healthCacheEntry{
		inputURL:   inputURL,
		outputURL:  strings.TrimSpace(svc.URL),
		status:     strings.ToLower(strings.TrimSpace(svc.Status)),
		responseMs: svc.ResponseMs,
		checkedAt:  now,
	}
	wsc.pruneHealthCache(now)
}

func (wsc *WebServiceCollector) pruneHealthCache(now time.Time) {
	if len(wsc.healthCache) <= maxHealthCacheEntries {
		return
	}

	for key, entry := range wsc.healthCache {
		if now.Sub(entry.checkedAt) > wsc.healthCacheTTL(entry.status) {
			delete(wsc.healthCache, key)
		}
	}
	if len(wsc.healthCache) <= maxHealthCacheEntries {
		return
	}

	for len(wsc.healthCache) > maxHealthCacheEntries {
		oldestKey := ""
		var oldestAt time.Time
		for key, entry := range wsc.healthCache {
			if oldestKey == "" || entry.checkedAt.Before(oldestAt) {
				oldestKey = key
				oldestAt = entry.checkedAt
			}
		}
		if oldestKey == "" {
			return
		}
		delete(wsc.healthCache, oldestKey)
	}
}

// enrichFromProxies queries all registered proxy providers concurrently, then
// enriches the discovered services with proxy route information.
func (wsc *WebServiceCollector) enrichFromProxies(containers []dockerpkg.DockerContainer, services []agentmgr.DiscoveredWebService) ([]agentmgr.DiscoveredWebService, bool) {
	type providerResult struct {
		name   string
		routes []ProxyRoute
		err    error
	}

	ch := make(chan providerResult, len(wsc.proxyProviders))

	for _, provider := range wsc.proxyProviders {
		go func(p proxypkg.Provider) {
			apiURL, ok := p.DetectAndConnect(containers)
			if !ok {
				ch <- providerResult{name: p.Name()}
				return
			}
			log.Printf("webservices: proxy/%s detected at %s", p.Name(), apiURL)
			routes, err := p.FetchRoutes(apiURL)
			ch <- providerResult{name: p.Name(), routes: routes, err: err}
		}(provider)
	}

	hadProviderError := false
	for i := 0; i < len(wsc.proxyProviders); i++ {
		result := <-ch
		if result.err != nil {
			hadProviderError = true
			log.Printf("webservices: proxy/%s error: %v", result.name, result.err)
			continue
		}
		if len(result.routes) > 0 {
			log.Printf("webservices: proxy/%s discovered %d routes", result.name, len(result.routes))
			services = enrichServicesWithRoutes(services, result.routes, result.name, wsc.assetID, wsc.hostIP, containers)
		}
	}

	return services, hadProviderError
}

// healthCheckWithFallback performs health check on a service.
// If the primary URL fails and a raw_url exists in metadata, retries with the raw URL
// to avoid false "down" from DNS resolution failures on proxied domains.
func (wsc *WebServiceCollector) healthCheckWithFallback(svc *agentmgr.DiscoveredWebService) {
	wsc.healthCheck(svc)

	// If proxied URL failed and we have a raw URL, retry with raw
	if svc.Status == "down" && svc.Metadata != nil && svc.Metadata["raw_url"] != "" {
		rawURL := svc.Metadata["raw_url"]
		healthPath := ""
		if svc.Metadata["health_path"] != "" {
			healthPath = svc.Metadata["health_path"]
		}
		result := wsc.probeHealthURL(rawURL, healthPath)
		if result.responded && result.status > 0 && result.status < 500 {
			svc.Status = "up"
			svc.ResponseMs = result.responseMs
		}
	}
}

// healthCheck performs an HTTP health check on a discovered service.
// It tries HEAD first, then falls back to GET. Status < 500 = "up".
func (wsc *WebServiceCollector) healthCheck(svc *agentmgr.DiscoveredWebService) {
	healthPath := ""
	if svc.Metadata != nil && svc.Metadata["health_path"] != "" {
		healthPath = svc.Metadata["health_path"]
	}

	result := wsc.probeHealthURL(svc.URL, healthPath)
	if result.responded {
		if result.baseURL != "" && result.baseURL != svc.URL {
			svc.URL = result.baseURL
		}
		svc.ResponseMs = result.responseMs
		if result.status > 0 && result.status < 500 {
			svc.Status = "up"
		} else {
			svc.Status = "down"
		}
		return
	}

	svc.ResponseMs = 0
	svc.Status = "down"
}

type healthProbeResult struct {
	baseURL    string
	status     int
	responseMs int
	responded  bool
}

// probeHealthURL checks both the provided scheme and its alternate (http<->https),
// then selects the most credible response. This improves scheme detection for
// non-standard TLS ports where static port heuristics are unreliable.
func (wsc *WebServiceCollector) probeHealthURL(baseURL, healthPath string) healthProbeResult {
	primary := wsc.probeCandidateHealth(baseURL, healthPath)
	tryAlternate := !primary.responded || primary.status == http.StatusBadRequest
	if !tryAlternate {
		return primary
	}

	altURL := alternateSchemeURL(baseURL)
	if altURL == "" {
		return primary
	}
	alternate := wsc.probeCandidateHealth(altURL, healthPath)
	return betterProbeResult(primary, alternate)
}

func (wsc *WebServiceCollector) probeCandidateHealth(baseURL, healthPath string) healthProbeResult {
	checkURL := baseURL
	if healthPath != "" {
		checkURL = strings.TrimRight(baseURL, "/") + healthPath
	}
	status, responseMs, responded := wsc.probeHTTP(checkURL)
	return healthProbeResult{
		baseURL:    baseURL,
		status:     status,
		responseMs: responseMs,
		responded:  responded,
	}
}

func betterProbeResult(primary, alternate healthProbeResult) healthProbeResult {
	primaryScore := probeScore(primary)
	alternateScore := probeScore(alternate)
	if alternateScore > primaryScore {
		return alternate
	}
	if primaryScore > alternateScore {
		return primary
	}

	// On equal confidence, prefer HTTPS for safer defaults.
	if primaryScore > 0 {
		primaryHTTPS := strings.HasPrefix(strings.ToLower(primary.baseURL), "https://")
		alternateHTTPS := strings.HasPrefix(strings.ToLower(alternate.baseURL), "https://")
		if alternateHTTPS && !primaryHTTPS {
			return alternate
		}
	}
	return primary
}

func probeScore(result healthProbeResult) int {
	if !result.responded {
		return 0
	}
	switch {
	case result.status >= 200 && result.status < 400:
		return 4
	case result.status == http.StatusUnauthorized || result.status == http.StatusForbidden || result.status == http.StatusNotFound:
		return 3
	case result.status == http.StatusBadRequest:
		return 1
	case result.status >= 400 && result.status < 500:
		return 2
	default:
		return 1
	}
}

func (wsc *WebServiceCollector) probeHTTP(url string) (status int, responseMs int, responded bool) {
	start := time.Now()
	status, ok := wsc.doHealthRequest(http.MethodHead, url)
	if ok {
		return status, int(time.Since(start).Milliseconds()), true
	}

	// HEAD failed — retry with GET (some services reject HEAD).
	start = time.Now()
	status, ok = wsc.doHealthRequest(http.MethodGet, url)
	if !ok {
		return 0, 0, false
	}
	return status, int(time.Since(start).Milliseconds()), true
}

func (wsc *WebServiceCollector) probeClientForURL(targetURL string) *http.Client {
	if wsc == nil {
		return nil
	}
	isHTTPS := strings.HasPrefix(strings.ToLower(strings.TrimSpace(targetURL)), "https://")
	if isHTTPS && wsc.insecureClient != nil {
		// Discovery probes do not carry secrets, so prefer the insecure client for HTTPS
		// to avoid cert-validation noise against self-signed/local endpoints.
		return wsc.insecureClient
	}
	return wsc.client
}

func (wsc *WebServiceCollector) fallbackProbeClientForURL(targetURL string, primary *http.Client) *http.Client {
	if wsc == nil {
		return nil
	}
	isHTTPS := strings.HasPrefix(strings.ToLower(strings.TrimSpace(targetURL)), "https://")
	if isHTTPS {
		if primary != wsc.insecureClient && wsc.insecureClient != nil {
			return wsc.insecureClient
		}
		if primary != wsc.client && wsc.client != nil {
			return wsc.client
		}
		return nil
	}
	if primary != wsc.client && wsc.client != nil {
		return wsc.client
	}
	return nil
}

// doHealthRequest makes an HTTP request and returns the status code and success flag.
func (wsc *WebServiceCollector) doHealthRequest(method, url string) (int, bool) {
	client := wsc.probeClientForURL(url)
	if client == nil {
		return 0, false
	}
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return 0, false
	}
	req.Header.Set("User-Agent", "LabTether-Agent/1.0")

	resp, err := client.Do(req) // #nosec G704 -- Request target already passed discovery/outbound validation before probing.
	if err != nil {
		if fallback := wsc.fallbackProbeClientForURL(url, client); fallback != nil {
			fallbackReq, reqErr := http.NewRequest(method, url, nil)
			if reqErr == nil {
				fallbackReq.Header.Set("User-Agent", "LabTether-Agent/1.0")
				resp, err = fallback.Do(fallbackReq) // #nosec G704 -- Fallback client reuses the same validated target URL.
			}
		}
	}
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()
	return resp.StatusCode, true
}

func alternateSchemeURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http":
		parsed.Scheme = "https"
	case "https":
		parsed.Scheme = "http"
	default:
		return ""
	}
	return parsed.String()
}

func cloneDiscoveredServices(in []agentmgr.DiscoveredWebService) []agentmgr.DiscoveredWebService {
	if len(in) == 0 {
		return nil
	}
	out := make([]agentmgr.DiscoveredWebService, 0, len(in))
	for _, svc := range in {
		cloned := svc
		if svc.Metadata != nil {
			cloned.Metadata = make(map[string]string, len(svc.Metadata))
			for key, value := range svc.Metadata {
				cloned.Metadata[key] = value
			}
		}
		out = append(out, cloned)
	}
	return out
}

func filterServicesBySource(in []agentmgr.DiscoveredWebService, source string) []agentmgr.DiscoveredWebService {
	if len(in) == 0 {
		return nil
	}
	out := make([]agentmgr.DiscoveredWebService, 0, len(in))
	for _, svc := range in {
		if svc.Source == source {
			out = append(out, svc)
		}
	}
	return out
}

func dedupeDiscoveredServices(in []agentmgr.DiscoveredWebService) []agentmgr.DiscoveredWebService {
	if len(in) == 0 {
		return in
	}
	out := make([]agentmgr.DiscoveredWebService, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, svc := range in {
		if svc.ID == "" {
			out = append(out, svc)
			continue
		}
		if _, ok := seen[svc.ID]; ok {
			continue
		}
		seen[svc.ID] = struct{}{}
		out = append(out, svc)
	}
	return out
}

// traefikHostRegex matches Traefik v2+ Host(`...`) rules.
var traefikHostRegex = regexp.MustCompile("Host\\(`([^`]+)`\\)")

// extractTraefikURL parses Traefik labels for a Host() routing rule and returns
// the corresponding URL. Returns empty string if no Traefik rule is found.
func extractTraefikURL(labels map[string]string) string {
	if labels == nil {
		return ""
	}

	// Look through all labels for Traefik router rules
	for key, val := range labels {
		if !strings.Contains(key, "traefik") {
			continue
		}
		if !strings.Contains(key, "rule") {
			continue
		}
		matches := traefikHostRegex.FindStringSubmatch(val)
		if len(matches) >= 2 {
			host := strings.TrimSpace(matches[1])
			if host != "" {
				return "https://" + host
			}
		}
	}
	return ""
}

// resolveHostIP attempts to determine the machine's outbound IP address
// without requiring Internet connectivity. Returns "localhost" on failure.
func ResolveHostIP() string {
	if ip := firstNonLoopbackHostIP(); ip != "" {
		return ip
	}
	return "localhost"
}

func firstNonLoopbackHostIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	// Prefer IPv4 private/LAN addresses first.
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, addrErr := iface.Addrs()
		if addrErr != nil {
			continue
		}
		for _, addr := range addrs {
			ip := ipFromAddr(addr)
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if v4 := ip.To4(); v4 != nil {
				return v4.String()
			}
		}
	}

	// Then allow non-loopback IPv6.
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, addrErr := iface.Addrs()
		if addrErr != nil {
			continue
		}
		for _, addr := range addrs {
			ip := ipFromAddr(addr)
			if ip == nil || ip.IsLoopback() || ip.To4() != nil {
				continue
			}
			return ip.String()
		}
	}

	return ""
}

func ipFromAddr(addr net.Addr) net.IP {
	switch value := addr.(type) {
	case *net.IPNet:
		return value.IP
	case *net.IPAddr:
		return value.IP
	default:
		return nil
	}
}
