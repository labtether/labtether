package agentcore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/servicehttp"
)

// StatusResponse is the unified status payload served by the /agent/status endpoint.
// The Swift mac-agent polls this every few seconds for the mini dashboard.
type StatusResponse struct {
	AgentName          string          `json:"agent_name"`
	AssetID            string          `json:"asset_id"`
	GroupID            string          `json:"group_id,omitempty"`
	Port               string          `json:"port"`
	DeviceFingerprint  string          `json:"device_fingerprint,omitempty"`
	DeviceKeyAlgorithm string          `json:"device_key_algorithm,omitempty"`
	Connected          bool            `json:"connected"`
	ConnectionState    string          `json:"connection_state"` // "connected", "connecting", "auth_failed", "disconnected"
	DisconnectedAt     *time.Time      `json:"disconnected_at,omitempty"`
	LastError          string          `json:"last_error,omitempty"`
	Uptime             string          `json:"uptime,omitempty"`
	StartedAt          time.Time       `json:"started_at"`
	Metrics            TelemetrySample `json:"metrics"`
	Alerts             []AlertSnapshot `json:"alerts"`
	AgentVersion       string          `json:"agent_version"`
	LocalBindAddress   string          `json:"local_bind_address,omitempty"`
	LocalAuthEnabled   bool            `json:"local_auth_enabled"`
	InsecureTransport  bool            `json:"allow_insecure_transport"`
	UpdateAvailable    bool            `json:"update_available"`
	LatestVersion      string          `json:"latest_version,omitempty"`
}

// AlertSnapshot is a point-in-time alert summary cached by the agent for local API clients.
type AlertSnapshot struct {
	ID        string    `json:"id"`
	Severity  string    `json:"severity"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary"`
	State     string    `json:"state"`
	Timestamp time.Time `json:"timestamp"`
}

// maxCachedAlerts is the upper bound on alerts kept in the local cache.
const maxCachedAlerts = 20

// statusHandler returns an http.HandlerFunc that serves the unified /agent/status response.
func (r *Runtime) statusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		sample := r.current()
		connected := r.transport != nil && r.transport.Connected()

		var connState, lastErr string
		var disconnectedAt *time.Time
		if r.transport != nil {
			state, errStr, discAt := r.transport.ConnectionState()
			connState = state
			lastErr = errStr
			if !discAt.IsZero() {
				disconnectedAt = &discAt
			}
		} else {
			connState = "disconnected"
		}

		var uptime string
		if !r.startedAt.IsZero() {
			uptime = time.Since(r.startedAt).Truncate(time.Second).String()
		}

		resp := StatusResponse{
			AgentName:         r.cfg.Name,
			AssetID:           r.cfg.AssetID,
			GroupID:           r.cfg.GroupID,
			Port:              r.cfg.Port,
			Connected:         connected,
			ConnectionState:   connState,
			DisconnectedAt:    disconnectedAt,
			LastError:         lastErr,
			Uptime:            uptime,
			StartedAt:         r.startedAt,
			Metrics:           sample,
			Alerts:            r.getAlerts(),
			AgentVersion:      r.cfg.Version,
			LocalBindAddress:  r.localBindAddress,
			LocalAuthEnabled:  r.localAuthEnabled,
			InsecureTransport: allowInsecureTransportOptIn(),
		}
		if r.deviceIdentity != nil {
			resp.DeviceFingerprint = r.deviceIdentity.Fingerprint
			resp.DeviceKeyAlgorithm = r.deviceIdentity.KeyAlgorithm
		}

		etagPayload, err := json.Marshal(statusResponseForETag(resp))
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to encode status etag payload")
			return
		}
		etag := statusPayloadETag(etagPayload)
		w.Header().Set("ETag", etag)
		if statusETagMatches(req.Header.Get("If-None-Match"), etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		servicehttp.WriteJSON(w, http.StatusOK, resp)
	}
}

func statusResponseForETag(resp StatusResponse) StatusResponse {
	// Uptime changes every request and would prevent conditional polling from
	// ever reusing status snapshots. Bucket it to 1 minute for ETag stability.
	resp.Uptime = statusUptimeBucket(resp.StartedAt)
	return resp
}

func statusUptimeBucket(startedAt time.Time) string {
	if startedAt.IsZero() {
		return ""
	}
	return time.Since(startedAt).Truncate(time.Minute).String()
}

func statusPayloadETag(payload []byte) string {
	sum := sha256.Sum256(payload)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

func statusETagMatches(headerValue, etag string) bool {
	canonicalETag := statusNormalizeETagToken(etag)
	if canonicalETag == "" {
		return false
	}
	for _, part := range strings.Split(headerValue, ",") {
		token := strings.TrimSpace(part)
		if token == "*" {
			return true
		}
		if statusNormalizeETagToken(token) == canonicalETag {
			return true
		}
	}
	return false
}

func statusNormalizeETagToken(value string) string {
	token := strings.TrimSpace(value)
	token = strings.TrimPrefix(token, "W/")
	token = strings.TrimPrefix(token, "w/")
	token = strings.TrimSpace(token)
	token = strings.Trim(token, `"`)
	return token
}

// getAlerts returns a copy of the cached alert snapshots.
func (r *Runtime) getAlerts() []AlertSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.alerts == nil {
		return []AlertSnapshot{}
	}
	out := make([]AlertSnapshot, len(r.alerts))
	copy(out, r.alerts)
	return out
}

// pushAlert adds or updates an alert in the local cache. If the alert ID already
// exists it is updated in place (upsert). The cache is capped at maxCachedAlerts,
// dropping the oldest entries when exceeded.
func (r *Runtime) pushAlert(alert AlertSnapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, a := range r.alerts {
		if a.ID == alert.ID {
			r.alerts[i] = alert
			return
		}
	}
	r.alerts = append(r.alerts, alert)
	if len(r.alerts) > maxCachedAlerts {
		r.alerts = r.alerts[len(r.alerts)-maxCachedAlerts:]
	}
}
