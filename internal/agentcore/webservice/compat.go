package webservice

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

const (
	compatMetadataConnector  = "compat_connector"
	compatMetadataConfidence = "compat_confidence"
	compatMetadataAuthHint   = "compat_auth_hint"
	compatMetadataProfile    = "compat_profile"
	compatMetadataEvidence   = "compat_evidence"

	compatMinConfidence = 0.60
	compatCacheHitTTL   = 10 * time.Minute
	compatCacheMissTTL  = 3 * time.Minute
)

type compatCacheEntry struct {
	match     compatMatch
	expiresAt time.Time
}

type compatMatch struct {
	connector  string
	profile    string
	authHint   string
	evidence   string
	confidence float64
	responded  bool

	serviceKey  string
	displayName string
	category    string
	iconKey     string
	healthPath  string
}

func (wsc *WebServiceCollector) applyCompatibilityMetadata(svc *agentmgr.DiscoveredWebService, baseURL string) {
	if svc == nil {
		return
	}
	if svc.Source == "manual" {
		return
	}
	if wsc.compatCache == nil {
		wsc.compatCache = make(map[string]compatCacheEntry)
	}

	if svc.Metadata == nil {
		svc.Metadata = make(map[string]string)
	}

	cacheKey := strings.ToLower(strings.TrimSpace(baseURL))
	if cacheKey == "" {
		cacheKey = strings.ToLower(strings.TrimSpace(svc.URL))
	}
	if cacheKey == "" {
		return
	}

	now := wsc.now()
	if cached, ok := wsc.compatCache[cacheKey]; ok {
		if now.Before(cached.expiresAt) {
			if cached.match.confidence >= compatMinConfidence {
				applyCompatMatchToService(svc, cached.match)
			}
			return
		}
		delete(wsc.compatCache, cacheKey)
	}

	match := matchCompatibilityFromServiceKey(strings.TrimSpace(svc.ServiceKey))
	if match.confidence < compatMinConfidence {
		match = wsc.detectCompatibleAPI(baseURL)
	}

	if match.confidence >= compatMinConfidence {
		wsc.compatCache[cacheKey] = compatCacheEntry{
			match:     match,
			expiresAt: now.Add(compatCacheHitTTL),
		}
		wsc.pruneCompatCache(now)
		applyCompatMatchToService(svc, match)
		return
	}

	wsc.compatCache[cacheKey] = compatCacheEntry{expiresAt: now.Add(compatCacheMissTTL)}
	wsc.pruneCompatCache(now)
}

func (wsc *WebServiceCollector) pruneCompatCache(now time.Time) {
	if len(wsc.compatCache) <= maxCompatCacheEntries {
		return
	}

	for key, entry := range wsc.compatCache {
		if now.After(entry.expiresAt) {
			delete(wsc.compatCache, key)
		}
	}
	if len(wsc.compatCache) <= maxCompatCacheEntries {
		return
	}

	for len(wsc.compatCache) > maxCompatCacheEntries {
		oldestKey := ""
		var oldestExpiry time.Time
		for key, entry := range wsc.compatCache {
			if oldestKey == "" || entry.expiresAt.Before(oldestExpiry) {
				oldestKey = key
				oldestExpiry = entry.expiresAt
			}
		}
		if oldestKey == "" {
			return
		}
		delete(wsc.compatCache, oldestKey)
	}
}

func matchCompatibilityFromServiceKey(serviceKey string) compatMatch {
	switch strings.ToLower(strings.TrimSpace(serviceKey)) {
	case "portainer":
		known, _ := LookupByKey("portainer")
		return compatMatch{
			connector:   "portainer",
			profile:     "service-key.portainer",
			authHint:    "api_key,password",
			evidence:    "known service key",
			confidence:  0.92,
			serviceKey:  known.Key,
			displayName: known.Name,
			category:    known.Category,
			iconKey:     known.IconKey,
			healthPath:  known.HealthPath,
		}
	case "homeassistant":
		known, _ := LookupByKey("homeassistant")
		return compatMatch{
			connector:   "homeassistant",
			profile:     "service-key.homeassistant",
			authHint:    "token",
			evidence:    "known service key",
			confidence:  0.92,
			serviceKey:  known.Key,
			displayName: known.Name,
			category:    known.Category,
			iconKey:     known.IconKey,
			healthPath:  known.HealthPath,
		}
	default:
		return compatMatch{}
	}
}

func (wsc *WebServiceCollector) detectCompatibleAPI(baseURL string) compatMatch {
	trimmedBase := strings.TrimSpace(baseURL)
	if trimmedBase == "" {
		return compatMatch{}
	}

	probeOrder := compatibilityProbeOrder(portFromURL(trimmedBase))
	best, responded := wsc.detectCompatibleAPIAtBase(trimmedBase, probeOrder)
	if best.confidence >= 0.97 || responded {
		return best
	}

	alternateBase := alternateSchemeURL(trimmedBase)
	if alternateBase == "" || strings.EqualFold(trimmedBase, alternateBase) {
		return best
	}

	alternateBest, _ := wsc.detectCompatibleAPIAtBase(alternateBase, probeOrder)
	if alternateBest.confidence > best.confidence {
		return alternateBest
	}
	return best
}

func (wsc *WebServiceCollector) detectCompatibleAPIAtBase(baseURL string, probeOrder []compatProbeFn) (compatMatch, bool) {
	best := compatMatch{}
	responded := false
	for _, probe := range probeOrder {
		match := probe(wsc, baseURL)
		if match.responded {
			responded = true
		}
		if match.confidence > best.confidence {
			best = match
			if best.confidence >= 0.97 {
				return best, responded
			}
		}
	}
	return best, responded
}

type compatProbeFn func(*WebServiceCollector, string) compatMatch

func compatibilityProbeOrder(port int) []compatProbeFn {
	switch port {
	case 8123:
		return []compatProbeFn{probeHomeAssistantAPI, probePortainerAPI, probeTrueNASAPI, probePBSAPI, probeProxmoxAPI, probeDockerEngineAPI}
	case 9443, 9000:
		return []compatProbeFn{probePortainerAPI, probeHomeAssistantAPI, probeTrueNASAPI, probePBSAPI, probeProxmoxAPI, probeDockerEngineAPI}
	case 8006:
		return []compatProbeFn{probeProxmoxAPI, probePBSAPI, probePortainerAPI, probeTrueNASAPI, probeHomeAssistantAPI, probeDockerEngineAPI}
	case 8007:
		return []compatProbeFn{probePBSAPI, probeProxmoxAPI, probePortainerAPI, probeTrueNASAPI, probeHomeAssistantAPI, probeDockerEngineAPI}
	case 2375, 2376:
		return []compatProbeFn{probeDockerEngineAPI, probePortainerAPI, probeHomeAssistantAPI, probeTrueNASAPI, probePBSAPI, probeProxmoxAPI}
	default:
		return []compatProbeFn{probePortainerAPI, probeHomeAssistantAPI, probeTrueNASAPI, probePBSAPI, probeProxmoxAPI, probeDockerEngineAPI}
	}
}

func probePortainerAPI(wsc *WebServiceCollector, baseURL string) compatMatch {
	responded := false
	if payload, status, ok := wsc.fetchJSON(baseURL, "/api/status"); ok {
		responded = true
		if status == http.StatusOK && payload != nil {
			if value := mapStringValue(payload, "Version", "version"); strings.TrimSpace(value) != "" {
				known, _ := LookupByKey("portainer")
				return compatMatch{
					connector:   "portainer",
					profile:     "portainer.api.status",
					authHint:    "api_key,password",
					evidence:    "api/status version",
					confidence:  0.97,
					responded:   true,
					serviceKey:  known.Key,
					displayName: known.Name,
					category:    known.Category,
					iconKey:     known.IconKey,
					healthPath:  known.HealthPath,
				}
			}
		}
	}
	if marker, markerResponded := wsc.pageContainsMarkerWithResponse(baseURL, "/", "portainer"); marker {
		known, _ := LookupByKey("portainer")
		return compatMatch{
			connector:   "portainer",
			profile:     "portainer.ui.marker",
			authHint:    "api_key,password",
			evidence:    "page marker",
			confidence:  0.83,
			responded:   true,
			serviceKey:  known.Key,
			displayName: known.Name,
			category:    known.Category,
			iconKey:     known.IconKey,
			healthPath:  known.HealthPath,
		}
	} else if markerResponded {
		responded = true
	}
	return compatMatch{responded: responded}
}

func probeHomeAssistantAPI(wsc *WebServiceCollector, baseURL string) compatMatch {
	responded := false
	if payload, status, ok := wsc.fetchJSON(baseURL, "/api"); ok {
		responded = true
		if status == http.StatusOK && payload != nil {
			if strings.Contains(strings.ToLower(mapStringValue(payload, "message")), "api running") {
				known, _ := LookupByKey("homeassistant")
				return compatMatch{
					connector:   "homeassistant",
					profile:     "homeassistant.api.root",
					authHint:    "token",
					evidence:    "api running message",
					confidence:  0.97,
					responded:   true,
					serviceKey:  known.Key,
					displayName: known.Name,
					category:    known.Category,
					iconKey:     known.IconKey,
					healthPath:  known.HealthPath,
				}
			}
		}
	}
	if marker, markerResponded := wsc.pageContainsMarkerWithResponse(baseURL, "/", "home assistant"); marker {
		known, _ := LookupByKey("homeassistant")
		return compatMatch{
			connector:   "homeassistant",
			profile:     "homeassistant.ui.marker",
			authHint:    "token",
			evidence:    "page marker",
			confidence:  0.82,
			responded:   true,
			serviceKey:  known.Key,
			displayName: known.Name,
			category:    known.Category,
			iconKey:     known.IconKey,
			healthPath:  known.HealthPath,
		}
	} else if markerResponded {
		responded = true
	}
	return compatMatch{responded: responded}
}

func probeTrueNASAPI(wsc *WebServiceCollector, baseURL string) compatMatch {
	responded := false
	if body, status, ok := wsc.fetchBody(baseURL, "/api/v2.0/system/version"); ok {
		responded = true
		lower := strings.ToLower(string(body))
		if status == http.StatusOK && (strings.Contains(lower, "truenas") || strings.Contains(lower, "freenas")) {
			return compatMatch{
				connector:   "truenas",
				profile:     "truenas.api.system.version",
				authHint:    "api_key",
				evidence:    "api/v2.0/system/version",
				confidence:  0.97,
				responded:   true,
				displayName: "TrueNAS",
				category:    CatStorage,
			}
		}
	}
	if marker, markerResponded := wsc.pageContainsMarkerWithResponse(baseURL, "/", "truenas"); marker {
		return compatMatch{
			connector:   "truenas",
			profile:     "truenas.ui.marker",
			authHint:    "api_key",
			evidence:    "page marker",
			confidence:  0.82,
			responded:   true,
			displayName: "TrueNAS",
			category:    CatStorage,
		}
	} else if markerResponded {
		responded = true
	}
	return compatMatch{responded: responded}
}

func probePBSAPI(wsc *WebServiceCollector, baseURL string) compatMatch {
	responded := false
	if marker, markerResponded := wsc.pageContainsMarkerWithResponse(baseURL, "/", "proxmox backup server"); marker {
		return compatMatch{
			connector:   "pbs",
			profile:     "pbs.ui.marker",
			authHint:    "api_token",
			evidence:    "page marker",
			confidence:  0.96,
			responded:   true,
			displayName: "Proxmox Backup Server",
			category:    CatStorage,
		}
	} else if markerResponded {
		responded = true
	}
	if payload, status, ok := wsc.fetchJSON(baseURL, "/api2/json/version"); ok {
		responded = true
		if status == http.StatusOK && payload != nil && hasProxmoxVersionShape(payload) {
			if strings.Contains(strings.ToLower(marshalMapToString(payload)), "backup") {
				return compatMatch{
					connector:   "pbs",
					profile:     "pbs.api2.version",
					authHint:    "api_token",
					evidence:    "api2 version payload",
					confidence:  0.88,
					responded:   true,
					displayName: "Proxmox Backup Server",
					category:    CatStorage,
				}
			}
		}
	}
	return compatMatch{responded: responded}
}

func probeProxmoxAPI(wsc *WebServiceCollector, baseURL string) compatMatch {
	responded := false
	if marker, markerResponded := wsc.pageContainsMarkerWithResponse(baseURL, "/", "proxmox virtual environment"); marker {
		return compatMatch{
			connector:   "proxmox",
			profile:     "proxmox.ui.marker",
			authHint:    "api_token,password",
			evidence:    "page marker",
			confidence:  0.96,
			responded:   true,
			displayName: "Proxmox VE",
			category:    CatManagement,
		}
	} else if markerResponded {
		responded = true
	}
	if payload, status, ok := wsc.fetchJSON(baseURL, "/api2/json/version"); ok {
		responded = true
		if status == http.StatusOK && payload != nil && hasProxmoxVersionShape(payload) && !strings.Contains(strings.ToLower(marshalMapToString(payload)), "backup") {
			return compatMatch{
				connector:   "proxmox",
				profile:     "proxmox.api2.version",
				authHint:    "api_token,password",
				evidence:    "api2 version payload",
				confidence:  0.74,
				responded:   true,
				displayName: "Proxmox VE",
				category:    CatManagement,
			}
		}
	}
	return compatMatch{responded: responded}
}

func probeDockerEngineAPI(wsc *WebServiceCollector, baseURL string) compatMatch {
	responded := false
	if payload, status, ok := wsc.fetchJSON(baseURL, "/version"); ok {
		responded = true
		if status == http.StatusOK && payload != nil && strings.TrimSpace(mapStringValue(payload, "ApiVersion")) != "" && strings.TrimSpace(mapStringValue(payload, "Version")) != "" {
			return compatMatch{
				connector:   "docker",
				profile:     "docker.api.version",
				authHint:    "none_or_mtls",
				evidence:    "docker /version payload",
				confidence:  0.96,
				responded:   true,
				displayName: "Docker Engine API",
				category:    CatManagement,
			}
		}
	}

	port := portFromURL(baseURL)
	if port == 2375 || port == 2376 {
		if body, status, ok := wsc.fetchBody(baseURL, "/_ping"); ok && status == http.StatusOK {
			responded = true
			if strings.Contains(strings.ToLower(string(body)), "ok") {
				return compatMatch{
					connector:   "docker",
					profile:     "docker.api.ping",
					authHint:    "none_or_mtls",
					evidence:    "docker /_ping",
					confidence:  0.86,
					responded:   true,
					displayName: "Docker Engine API",
					category:    CatManagement,
				}
			}
		} else if ok {
			responded = true
		}
	}
	return compatMatch{responded: responded}
}

func hasProxmoxVersionShape(payload map[string]any) bool {
	dataRaw, ok := payload["data"]
	if !ok {
		return false
	}
	data, ok := dataRaw.(map[string]any)
	if !ok {
		return false
	}
	version := strings.TrimSpace(mapStringValue(data, "version"))
	release := strings.TrimSpace(mapStringValue(data, "release"))
	return version != "" || release != ""
}

func mapStringValue(values map[string]any, keys ...string) string {
	if len(values) == 0 {
		return ""
	}
	for _, key := range keys {
		for existing, raw := range values {
			if !strings.EqualFold(strings.TrimSpace(existing), strings.TrimSpace(key)) {
				continue
			}
			if asString, ok := raw.(string); ok {
				return strings.TrimSpace(asString)
			}
		}
	}
	return ""
}

func marshalMapToString(values map[string]any) string {
	if len(values) == 0 {
		return ""
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func applyCompatMatchToService(svc *agentmgr.DiscoveredWebService, match compatMatch) {
	if svc == nil || match.confidence < compatMinConfidence || strings.TrimSpace(match.connector) == "" {
		return
	}
	if svc.Metadata == nil {
		svc.Metadata = make(map[string]string)
	}

	svc.Metadata[compatMetadataConnector] = strings.TrimSpace(match.connector)
	svc.Metadata[compatMetadataConfidence] = strconv.FormatFloat(match.confidence, 'f', 2, 64)
	if strings.TrimSpace(match.authHint) != "" {
		svc.Metadata[compatMetadataAuthHint] = strings.TrimSpace(match.authHint)
	}
	if strings.TrimSpace(match.profile) != "" {
		svc.Metadata[compatMetadataProfile] = strings.TrimSpace(match.profile)
	}
	if strings.TrimSpace(match.evidence) != "" {
		svc.Metadata[compatMetadataEvidence] = strings.TrimSpace(match.evidence)
	}

	if strings.TrimSpace(svc.ServiceKey) == "" && strings.TrimSpace(match.serviceKey) != "" {
		svc.ServiceKey = strings.TrimSpace(match.serviceKey)
	}
	if strings.TrimSpace(match.displayName) != "" {
		if strings.TrimSpace(svc.Name) == "" || isGenericPortServiceName(svc.Name) {
			svc.Name = strings.TrimSpace(match.displayName)
		}
	}
	if strings.TrimSpace(match.category) != "" {
		if strings.TrimSpace(svc.Category) == "" || strings.EqualFold(strings.TrimSpace(svc.Category), CatOther) {
			svc.Category = strings.TrimSpace(match.category)
		}
	}
	if strings.TrimSpace(match.iconKey) != "" && strings.TrimSpace(svc.IconKey) == "" {
		svc.IconKey = strings.TrimSpace(match.iconKey)
	}
	if strings.TrimSpace(match.healthPath) != "" && strings.TrimSpace(svc.Metadata["health_path"]) == "" {
		svc.Metadata["health_path"] = strings.TrimSpace(match.healthPath)
	}
}

func isGenericPortServiceName(name string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(name))
	if !strings.HasPrefix(trimmed, "port ") {
		return false
	}
	_, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(trimmed, "port ")))
	return err == nil
}
