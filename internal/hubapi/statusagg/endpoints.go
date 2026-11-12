package statusagg

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/runtimesettings"
	"github.com/labtether/labtether/internal/securityruntime"
)

const (
	statusRoutingBaseURLCacheTTL = 30 * time.Second
	statusEndpointProbeCacheTTL  = 15 * time.Second
	hubOwnProbeTimeout           = 3 * time.Second
)

// probeHTTPClient is a dedicated client for endpoint health probes with
// aggressive timeouts to prevent connection pool exhaustion from slow targets.
var probeHTTPClient = &http.Client{
	Transport: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
		ResponseHeaderTimeout: 3 * time.Second,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       30 * time.Second,
	},
	Timeout: 5 * time.Second,
}

// ProbeEndpointFunc is the function used to probe a single endpoint. It is a
// package variable so tests can replace it without starting real HTTP servers.
var ProbeEndpointFunc = probeEndpoint

// collectEndpointResults resolves routing URLs, builds the target list, and
// returns cached or freshly probed results.
func (d *Deps) collectEndpointResults(ctx context.Context) []EndpointResult {
	apiBaseURL, agentBaseURL := d.routingBaseURLs()
	targets := buildEndpointTargets(apiBaseURL, agentBaseURL, dockerHostedHubRuntime())
	return d.probeEndpointsCached(ctx, targets)
}

// BuildEndpointTargets is the exported version for use by cmd/labtether tests.
func BuildEndpointTargets(
	apiBaseURL, agentBaseURL ResolvedRoutingURL,
	dockerHostedHub bool,
) []endpointTarget {
	return buildEndpointTargets(apiBaseURL, agentBaseURL, dockerHostedHub)
}

func buildEndpointTargets(
	apiBaseURL, agentBaseURL ResolvedRoutingURL,
	dockerHostedHub bool,
) []endpointTarget {
	targets := []endpointTarget{{
		Name:          "LabTether",
		URL:           statusHealthURL(apiBaseURL.URL),
		HubOwnAddress: deployedAddress(apiBaseURL.Source),
	}}
	// The Node Agent endpoint is only relevant for Docker-hosted hub runtime.
	// In local dev/non-container runs, stale UI overrides can otherwise surface
	// false "Node Agent endpoint is down" warnings in dashboard narrative text.
	if dockerHostedHub && agentBaseURL.Source != runtimesettings.SourceDefault {
		targets = append(targets, endpointTarget{
			Name:          "Node Agent",
			URL:           statusHealthURL(agentBaseURL.URL),
			HubOwnAddress: deployedAddress(agentBaseURL.Source),
		})
	}
	return targets
}

// deployedAddress reports whether a routing URL came with the deployment: an
// environment variable or the built-in default. A value saved in Settings
// does not count.
func deployedAddress(source runtimesettings.Source) bool {
	return source == runtimesettings.SourceDocker || source == runtimesettings.SourceDefault
}

func dockerHostedHubRuntime() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

func statusHealthURL(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return ""
	}
	return trimmed + "/healthz"
}

// RoutingBaseURLs is the exported version of routingBaseURLs, used by
// cmd/labtether tests via the bridge.
func (d *Deps) RoutingBaseURLs() (ResolvedRoutingURL, ResolvedRoutingURL) {
	return d.routingBaseURLs()
}

func (d *Deps) routingBaseURLs() (ResolvedRoutingURL, ResolvedRoutingURL) {
	now := time.Now().UTC()
	d.Cache.RoutingBaseURLCacheMu.RLock()
	cacheEntry := d.Cache.RoutingBaseURLCache
	d.Cache.RoutingBaseURLCacheMu.RUnlock()
	if cacheEntry.ExpiresAt.After(now) {
		return cacheEntry.APIBaseURL, cacheEntry.AgentBaseURL
	}

	apiBaseURL, agentBaseURL := d.routingBaseURLsUncached()

	d.Cache.RoutingBaseURLCacheMu.Lock()
	d.Cache.RoutingBaseURLCache = RoutingBaseURLCacheEntry{
		ExpiresAt:    now.Add(statusRoutingBaseURLCacheTTL),
		APIBaseURL:   apiBaseURL,
		AgentBaseURL: agentBaseURL,
	}
	d.Cache.RoutingBaseURLCacheMu.Unlock()

	return apiBaseURL, agentBaseURL
}

func (d *Deps) routingBaseURLsUncached() (ResolvedRoutingURL, ResolvedRoutingURL) {
	overrides := map[string]string{}
	if d.RuntimeStore != nil {
		listed, err := d.RuntimeStore.ListRuntimeSettingOverrides()
		if err != nil {
			log.Printf("status aggregate: failed to load runtime overrides: %v", err)
		} else {
			overrides = listed
		}
	}
	apiBaseURL := resolveRoutingURL(runtimesettings.KeyRoutingAPIBaseURL, overrides)
	agentBaseURL := resolveRoutingURL(runtimesettings.KeyRoutingAgentBaseURL, overrides)
	return apiBaseURL, agentBaseURL
}

// ProbeEndpointsCached is the exported version of probeEndpointsCached, used
// by cmd/labtether tests via the bridge.
func (d *Deps) ProbeEndpointsCached(ctx context.Context, targets []EndpointTarget) []EndpointResult {
	return d.probeEndpointsCached(ctx, targets)
}

func (d *Deps) probeEndpointsCached(ctx context.Context, targets []endpointTarget) []EndpointResult {
	now := time.Now().UTC()
	fingerprint := endpointTargetsFingerprint(targets)
	d.Cache.EndpointProbeCacheMu.RLock()
	cacheEntry := d.Cache.EndpointProbeCache
	d.Cache.EndpointProbeCacheMu.RUnlock()
	if cacheEntry.TargetsFingerprint == fingerprint && cacheEntry.ExpiresAt.After(now) {
		return cloneEndpointResults(cacheEntry.Results)
	}

	// Deduplicate concurrent probe requests via singleflight to prevent
	// thundering herd when multiple console clients trigger cache expiry.
	val, _, _ := d.Cache.EndpointProbeGroup.Do("probes", func() (any, error) {
		// Re-check cache inside singleflight — another caller may have populated it.
		d.Cache.EndpointProbeCacheMu.RLock()
		entry := d.Cache.EndpointProbeCache
		d.Cache.EndpointProbeCacheMu.RUnlock()
		if entry.TargetsFingerprint == fingerprint && entry.ExpiresAt.After(time.Now().UTC()) {
			return cloneEndpointResults(entry.Results), nil
		}

		results := make([]EndpointResult, len(targets))
		var wg sync.WaitGroup
		for index, target := range targets {
			wg.Add(1)
			go func(idx int, probeTarget endpointTarget) {
				defer wg.Done()
				results[idx] = ProbeEndpointFunc(ctx, probeTarget)
			}(index, target)
		}
		wg.Wait()

		d.Cache.EndpointProbeCacheMu.Lock()
		d.Cache.EndpointProbeCache = EndpointProbeCacheEntry{
			TargetsFingerprint: fingerprint,
			ExpiresAt:          time.Now().UTC().Add(statusEndpointProbeCacheTTL),
			Results:            cloneEndpointResults(results),
		}
		d.Cache.EndpointProbeCacheMu.Unlock()

		return results, nil
	})
	return cloneEndpointResults(val.([]EndpointResult))
}

func endpointTargetsFingerprint(targets []endpointTarget) string {
	if len(targets) == 0 {
		return "none"
	}
	var builder strings.Builder
	for idx, target := range targets {
		if idx > 0 {
			builder.WriteByte('|')
		}
		builder.WriteString(strings.TrimSpace(target.Name))
		builder.WriteByte('@')
		builder.WriteString(strings.TrimSpace(target.URL))
		if target.HubOwnAddress {
			builder.WriteString("#own")
		}
	}
	return builder.String()
}

func cloneEndpointResults(results []EndpointResult) []EndpointResult {
	if len(results) == 0 {
		return []EndpointResult{}
	}
	return append([]EndpointResult(nil), results...)
}

func resolveRoutingURL(key string, overrides map[string]string) ResolvedRoutingURL {
	definition, ok := runtimesettings.DefinitionByKey(key)
	if !ok {
		return ResolvedRoutingURL{}
	}

	envValue := runtimesettings.ResolveEnvValue(definition, os.Getenv)
	overrideValue := ""
	if rawOverride, hasOverride := overrides[key]; hasOverride {
		normalized, err := runtimesettings.NormalizeValue(definition, rawOverride)
		if err == nil {
			overrideValue = normalized
		}
	}

	effective, source := runtimesettings.EffectiveValue(definition, envValue, overrideValue)
	return ResolvedRoutingURL{
		URL:    strings.TrimRight(strings.TrimSpace(effective), "/"),
		Source: source,
	}
}

func probeEndpoint(ctx context.Context, target endpointTarget) EndpointResult {
	result := EndpointResult{
		Name:   target.Name,
		URL:    target.URL,
		OK:     false,
		Status: "down",
	}
	if strings.TrimSpace(target.URL) == "" {
		result.Error = "endpoint not configured"
		return result
	}

	startedAt := time.Now()
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resp, err := sendProbe(probeCtx, target)
	result.LatencyMs = time.Since(startedAt).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))

	result.Code = resp.StatusCode
	result.OK = resp.StatusCode >= 200 && resp.StatusCode < 300
	if result.OK {
		result.Status = "up"
	}
	return result
}

// sendProbe sends the health request through the outbound policy. The one
// exception is the hub's own deployed address over plain http, which is what
// a hub has when a reverse proxy ends TLS (LABTETHER_TLS_MODE=disabled): that
// request can reach only that origin, on this host or a private network.
func sendProbe(ctx context.Context, target endpointTarget) (*http.Response, error) {
	if target.HubOwnAddress && hasPlainHTTPScheme(target.URL) {
		return securityruntime.GetHubOwnHTTP(ctx, target.URL, hubOwnProbeTimeout)
	}
	req, err := securityruntime.NewOutboundRequestWithContext(ctx, http.MethodGet, target.URL, nil)
	if err != nil {
		return nil, err
	}
	return securityruntime.DoOutboundRequest(probeHTTPClient, req)
}

func hasPlainHTTPScheme(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	return err == nil && strings.EqualFold(parsed.Scheme, "http")
}
