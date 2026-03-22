package webservice

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

const (
	maxFingerprintBodyBytes = 64 * 1024
	fingerprintCacheHitTTL  = 15 * time.Minute
	fingerprintCacheMissTTL = 5 * time.Minute

	labtetherServiceKey = "labtether"
	labtetherConsole    = "console"
	labtetherAPI        = "api"
)

type fingerprintCacheEntry struct {
	known     KnownService
	found     bool
	expiresAt time.Time
}

func (wsc *WebServiceCollector) applyFingerprintMetadata(svc *agentmgr.DiscoveredWebService) {
	if svc == nil {
		return
	}
	if svc.Source != "scan" && svc.Source != "proxy" && svc.Source != "docker" {
		return
	}

	baseURL := serviceFingerprintBaseURL(*svc)
	if baseURL == "" {
		return
	}

	if strings.TrimSpace(svc.ServiceKey) == "" {
		known, ok := wsc.cachedFingerprintKnownService(baseURL)
		if ok {
			svc.ServiceKey = known.Key
			svc.Name = known.Name
			svc.Category = known.Category
			svc.IconKey = known.IconKey

			if known.HealthPath != "" {
				if svc.Metadata == nil {
					svc.Metadata = make(map[string]string)
				}
				if strings.TrimSpace(svc.Metadata["health_path"]) == "" {
					svc.Metadata["health_path"] = known.HealthPath
				}
			}
		}
	}

	wsc.applyCompatibilityMetadata(svc, baseURL)
}

func (wsc *WebServiceCollector) cachedFingerprintKnownService(baseURL string) (KnownService, bool) {
	normalized := strings.ToLower(strings.TrimSpace(baseURL))
	if normalized == "" {
		return KnownService{}, false
	}
	if wsc.fingerprintCache == nil {
		wsc.fingerprintCache = make(map[string]fingerprintCacheEntry)
	}

	now := wsc.now()
	if cached, ok := wsc.fingerprintCache[normalized]; ok {
		if now.Before(cached.expiresAt) {
			return cached.known, cached.found
		}
		delete(wsc.fingerprintCache, normalized)
	}

	known, found := wsc.fingerprintKnownService(baseURL)
	ttl := fingerprintCacheMissTTL
	if found {
		ttl = fingerprintCacheHitTTL
	}
	wsc.fingerprintCache[normalized] = fingerprintCacheEntry{
		known:     known,
		found:     found,
		expiresAt: now.Add(ttl),
	}
	wsc.pruneFingerprintCache(now)
	return known, found
}

func (wsc *WebServiceCollector) pruneFingerprintCache(now time.Time) {
	if len(wsc.fingerprintCache) <= maxFingerprintCacheEntries {
		return
	}

	for key, entry := range wsc.fingerprintCache {
		if now.After(entry.expiresAt) {
			delete(wsc.fingerprintCache, key)
		}
	}
	if len(wsc.fingerprintCache) <= maxFingerprintCacheEntries {
		return
	}

	for len(wsc.fingerprintCache) > maxFingerprintCacheEntries {
		oldestKey := ""
		var oldestExpiry time.Time
		for key, entry := range wsc.fingerprintCache {
			if oldestKey == "" || entry.expiresAt.Before(oldestExpiry) {
				oldestKey = key
				oldestExpiry = entry.expiresAt
			}
		}
		if oldestKey == "" {
			return
		}
		delete(wsc.fingerprintCache, oldestKey)
	}
}

func serviceFingerprintBaseURL(svc agentmgr.DiscoveredWebService) string {
	baseURL := strings.TrimSpace(svc.URL)
	if svc.Metadata == nil {
		return baseURL
	}
	if rawURL := strings.TrimSpace(svc.Metadata["raw_url"]); rawURL != "" {
		baseURL = rawURL
	}
	if backendURL := strings.TrimSpace(svc.Metadata["backend_url"]); backendURL != "" {
		baseURL = backendURL
	}
	return baseURL
}

func (wsc *WebServiceCollector) fingerprintKnownService(baseURL string) (KnownService, bool) {
	if wsc.isLabTetherService(baseURL) {
		if known, ok := LookupByKey("labtether"); ok {
			return known, true
		}
	}

	if key, responded := wsc.fingerprintByHTTPWithResponse(baseURL); key != "" {
		if known, ok := LookupByKey(key); ok {
			return known, true
		}
	} else if !responded {
		if alternateBase := alternateSchemeURL(baseURL); alternateBase != "" && !strings.EqualFold(baseURL, alternateBase) {
			if key, _ := wsc.fingerprintByHTTPWithResponse(alternateBase); key != "" {
				if known, ok := LookupByKey(key); ok {
					return known, true
				}
			}
		}
	}

	return KnownService{}, false
}

// htmlFingerprintEntry maps a service key to HTML markers found in the root page.
type htmlFingerprintEntry struct {
	serviceKey string
	markers    []string // any marker matching (case-insensitive) identifies the service
}

// htmlFingerprints is the ordered table of HTML-based fingerprints.
// More specific markers come first to avoid false matches (e.g. PBS before PVE).
var htmlFingerprints = []htmlFingerprintEntry{
	// NAS devices — typically discovered via port scan or proxy with opaque names.
	{"truenas", []string{"truenas"}},
	{"synology", []string{"synology", "diskstation"}},
	{"qnap", []string{"qnap"}},
	// Hypervisors — check PBS before PVE since "proxmox" alone matches both.
	{"proxmox-backup", []string{"proxmox backup server"}},
	{"proxmox", []string{"proxmox virtual environment", "pve manager"}},
	// Network appliances.
	{"pfsense", []string{"pfsense"}},
	{"opnsense", []string{"opnsense"}},
	// Common self-hosted services that may run natively (not Docker).
	{"homeassistant", []string{"home-assistant"}},
	{"pihole", []string{"pi-hole"}},
	{"unifi", []string{"unifi network", "ubiquiti"}},
	{"cockpit", []string{"/cockpit/", "cockpit-ws"}},
}

func (wsc *WebServiceCollector) fingerprintByHTTPWithResponse(baseURL string) (string, bool) {
	// Phase 1: Fetch root page and check HTML markers.
	body, rootStatus, rootOK := wsc.fetchBody(baseURL, "/")
	if !rootOK {
		return "", false // unreachable, skip further probes
	}
	responded := true
	if rootStatus < 500 {
		if key := matchHTMLFingerprint(body); key != "" {
			return key, true
		}
	}

	// Phase 2: If root was a redirect, check the /ui/ path (TrueNAS redirects there).
	if rootStatus >= 300 && rootStatus < 400 {
		if redirectBody, status, ok := wsc.fetchBody(baseURL, "/ui/"); ok && status < 500 {
			responded = true
			if key := matchHTMLFingerprint(redirectBody); key != "" {
				return key, true
			}
		}
	}

	// Phase 3: Service-specific API endpoint probes.
	key, apiResponded := wsc.fingerprintByAPIWithResponse(baseURL)
	return key, responded || apiResponded
}

// matchHTMLFingerprint checks a page body against all known HTML markers.
func matchHTMLFingerprint(body []byte) string {
	lower := strings.ToLower(string(body))
	for _, fp := range htmlFingerprints {
		for _, marker := range fp.markers {
			if strings.Contains(lower, marker) {
				return fp.serviceKey
			}
		}
	}
	return ""
}

func (wsc *WebServiceCollector) fingerprintByAPIWithResponse(baseURL string) (string, bool) {
	responded := false

	// TrueNAS: unique /api/v2.0/ prefix (returns 200/401/403, never 404 on real TrueNAS).
	if _, status, ok := wsc.fetchBody(baseURL, "/api/v2.0/system/version"); ok {
		responded = true
		if status != http.StatusNotFound {
			return "truenas", true
		}
	}
	// Synology DSM: unique /webapi/query.cgi endpoint.
	if payload, status, ok := wsc.fetchJSON(baseURL, "/webapi/query.cgi?api=SYNO.API.Info&version=1&method=query"); ok {
		responded = true
		if status == http.StatusOK && payload != nil {
			if _, hasData := payload["data"]; hasData {
				return "synology", true
			}
		}
	}
	// QNAP QTS: unique /cgi-bin/authLogin.cgi with QNAP-specific response content.
	if qnapBody, status, ok := wsc.fetchBody(baseURL, "/cgi-bin/authLogin.cgi"); ok && status < 500 {
		responded = true
		lower := strings.ToLower(string(qnapBody))
		if strings.Contains(lower, "qnap") || strings.Contains(lower, "authsid") {
			return "qnap", true
		}
	}
	return "", responded
}

func (wsc *WebServiceCollector) isLabTetherService(baseURL string) bool {
	found, responded := wsc.isLabTetherServiceAtBase(baseURL)
	if found || responded {
		return found
	}

	alternateBase := alternateSchemeURL(baseURL)
	if alternateBase == "" || strings.EqualFold(baseURL, alternateBase) {
		return false
	}
	found, _ = wsc.isLabTetherServiceAtBase(alternateBase)
	return found
}

func (wsc *WebServiceCollector) isLabTetherServiceAtBase(baseURL string) (bool, bool) {
	responded := false

	if body, status, ok := wsc.fetchBody(baseURL, "/healthz"); ok {
		responded = true
		if status == http.StatusOK {
			payload := make(map[string]any)
			if err := json.Unmarshal(body, &payload); err == nil && hasLabTetherServiceField(payload) {
				return true, true
			}
		}
	}

	if body, status, ok := wsc.fetchBody(baseURL, "/version"); ok {
		responded = true
		if status == http.StatusOK {
			payload := make(map[string]any)
			if err := json.Unmarshal(body, &payload); err == nil && hasLabTetherServiceField(payload) {
				return true, true
			}
		}
	}

	if body, _, ok := wsc.fetchBody(baseURL, "/api/health"); ok {
		responded = true
		payload := make(map[string]any)
		if err := json.Unmarshal(body, &payload); err == nil && hasLabTetherFrontendHealthPayload(payload) {
			loginMarker, loginResponded := wsc.pageContainsMarkerWithResponse(baseURL, "/login", "labtether")
			rootMarker, rootResponded := wsc.pageContainsMarkerWithResponse(baseURL, "/", "labtether")
			responded = responded || loginResponded || rootResponded
			if loginMarker || rootMarker {
				return true, true
			}
		}
	}

	loginMarker, loginResponded := wsc.pageContainsMarkerWithResponse(baseURL, "/login", "labtether")
	responded = responded || loginResponded
	if loginMarker {
		authLogin, authResponded := wsc.hasLabTetherAuthLoginEndpointWithResponse(baseURL)
		responded = responded || authResponded
		if authLogin {
			return true, true
		}
	}

	return false, responded
}

func hasLabTetherServiceField(payload map[string]any) bool {
	service, _ := payload["service"].(string)
	return strings.EqualFold(strings.TrimSpace(service), "labtether")
}

func hasLabTetherFrontendHealthPayload(payload map[string]any) bool {
	status, ok := payload["status"].(string)
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "ok", "error":
		return true
	default:
		return false
	}
}

func (wsc *WebServiceCollector) pageContainsMarkerWithResponse(baseURL, path, marker string) (bool, bool) {
	body, status, ok := wsc.fetchBody(baseURL, path)
	if !ok || status >= http.StatusInternalServerError {
		return false, false
	}
	return strings.Contains(strings.ToLower(string(body)), strings.ToLower(marker)), true
}

func (wsc *WebServiceCollector) hasLabTetherAuthLoginEndpointWithResponse(baseURL string) (bool, bool) {
	_, status, ok := wsc.fetchBody(baseURL, "/api/auth/login")
	if !ok {
		return false, false
	}
	switch status {
	case http.StatusMethodNotAllowed, http.StatusBadRequest, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity:
		return true, true
	default:
		return false, true
	}
}

func normalizeLabTetherServices(services []agentmgr.DiscoveredWebService) {
	if len(services) == 0 {
		return
	}

	hostHasConsole := make(map[string]bool)

	for i := range services {
		svc := &services[i]
		if strings.TrimSpace(svc.ServiceKey) != labtetherServiceKey {
			continue
		}

		component := classifyLabTetherComponent(*svc)
		if component == "" {
			continue
		}

		if svc.Metadata == nil {
			svc.Metadata = make(map[string]string)
		}
		svc.Metadata["labtether_component"] = component

		switch component {
		case labtetherConsole:
			svc.Name = "LabTether Console"
			hostHasConsole[strings.TrimSpace(svc.HostAssetID)] = true
			delete(svc.Metadata, "hidden")
		case labtetherAPI:
			svc.Name = "LabTether API"
		}
	}

	for i := range services {
		svc := &services[i]
		if strings.TrimSpace(svc.ServiceKey) != labtetherServiceKey || svc.Metadata == nil {
			continue
		}
		if svc.Metadata["labtether_component"] != labtetherAPI {
			continue
		}
		if hostHasConsole[strings.TrimSpace(svc.HostAssetID)] {
			svc.Metadata["hidden"] = "true"
		} else {
			delete(svc.Metadata, "hidden")
		}
	}
}

func classifyLabTetherComponent(svc agentmgr.DiscoveredWebService) string {
	if strings.TrimSpace(svc.ServiceKey) != labtetherServiceKey {
		return ""
	}

	if svc.Metadata != nil {
		switch strings.ToLower(strings.TrimSpace(svc.Metadata["labtether_component"])) {
		case labtetherConsole:
			return labtetherConsole
		case labtetherAPI:
			return labtetherAPI
		}
	}

	port := portFromURL(svc.URL)
	if port == 0 && svc.Metadata != nil {
		if rawURL := strings.TrimSpace(svc.Metadata["raw_url"]); rawURL != "" {
			port = portFromURL(rawURL)
		}
	}
	if port == 0 && svc.Metadata != nil {
		if backendURL := strings.TrimSpace(svc.Metadata["backend_url"]); backendURL != "" {
			port = portFromURL(backendURL)
		}
	}

	switch port {
	case 3000:
		return labtetherConsole
	case 8080, 8443:
		return labtetherAPI
	}

	if svc.Metadata != nil {
		healthPath := strings.TrimSpace(svc.Metadata["health_path"])
		switch healthPath {
		case "/api/health":
			return labtetherConsole
		case "/healthz", "/version":
			return labtetherAPI
		}
	}

	return ""
}

func (wsc *WebServiceCollector) fetchJSON(baseURL, path string) (map[string]any, int, bool) {
	body, status, ok := wsc.fetchBody(baseURL, path)
	if !ok {
		return nil, status, false
	}
	payload := make(map[string]any)
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, status, true
	}
	return payload, status, true
}

func (wsc *WebServiceCollector) fetchBody(baseURL, path string) ([]byte, int, bool) {
	targetURL := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
	return wsc.doBodyRequest(http.MethodGet, targetURL)
}

func (wsc *WebServiceCollector) doBodyRequest(method, targetURL string) ([]byte, int, bool) {
	client := wsc.probeClientForURL(targetURL)
	if client == nil {
		return nil, 0, false
	}
	req, err := http.NewRequest(method, targetURL, nil)
	if err != nil {
		return nil, 0, false
	}
	req.Header.Set("User-Agent", "LabTether-Agent/1.0")

	resp, err := client.Do(req) // #nosec G704 -- Request target already passed discovery/outbound validation before fingerprinting.
	if err != nil {
		if fallback := wsc.fallbackProbeClientForURL(targetURL, client); fallback != nil {
			fallbackReq, reqErr := http.NewRequest(method, targetURL, nil)
			if reqErr == nil {
				fallbackReq.Header.Set("User-Agent", "LabTether-Agent/1.0")
				resp, err = fallback.Do(fallbackReq) // #nosec G704 -- Fallback client reuses the same validated target URL.
			}
		}
	}
	if err != nil {
		return nil, 0, false
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxFingerprintBodyBytes))
	if readErr != nil {
		return nil, resp.StatusCode, false
	}
	return body, resp.StatusCode, true
}
