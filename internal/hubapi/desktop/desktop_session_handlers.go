package desktop

import (
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

// DesktopSessionOptions holds per-session desktop configuration.
type DesktopSessionOptions struct {
	Protocol       string
	Quality        string
	Display        string
	Record         bool
	VNCPassword    string
	FallbackReason string
}

// DesktopSPICEProxyTarget holds SPICE proxy connection details for a session.
type DesktopSPICEProxyTarget struct {
	Host       string
	TLSPort    int
	Password   string // #nosec G117 -- Session credential is generated or supplied at runtime, not hardcoded.
	Type       string
	CA         string
	Proxy      string
	SkipVerify bool
}

// NormalizeDesktopProtocol normalizes a protocol string.
func NormalizeDesktopProtocol(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "webrtc":
		return "webrtc"
	case "rdp":
		return "rdp"
	case "spice":
		return "spice"
	default:
		return "vnc"
	}
}

const (
	vncSessionPasswordLength = 8
	vncPasswordAlphabet      = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
)

func generateSessionVNCPassword() (string, error) {
	buf := make([]byte, vncSessionPasswordLength)
	if _, err := cryptorand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, vncSessionPasswordLength)
	for i, b := range buf {
		out[i] = vncPasswordAlphabet[int(b)%len(vncPasswordAlphabet)]
	}
	return string(out), nil
}

func (d *Deps) SetDesktopSessionOptions(sessionID string, opts DesktopSessionOptions) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	d.DesktopSessionMu.Lock()
	if *d.DesktopSessionOpts == nil {
		*d.DesktopSessionOpts = make(map[string]DesktopSessionOptions, 64)
	}
	(*d.DesktopSessionOpts)[sessionID] = opts
	d.DesktopSessionMu.Unlock()
}

func (d *Deps) GetDesktopSessionOptions(sessionID string) DesktopSessionOptions {
	d.DesktopSessionMu.RLock()
	defer d.DesktopSessionMu.RUnlock()
	if *d.DesktopSessionOpts == nil {
		return DesktopSessionOptions{}
	}
	return (*d.DesktopSessionOpts)[strings.TrimSpace(sessionID)]
}

func (d *Deps) ClearDesktopSessionOptions(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	d.DesktopSessionMu.Lock()
	if *d.DesktopSessionOpts != nil {
		delete(*d.DesktopSessionOpts, sessionID)
	}
	d.DesktopSessionMu.Unlock()
}

func (d *Deps) SetDesktopSPICEProxyTarget(sessionID string, target DesktopSPICEProxyTarget) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	d.DesktopSPICEMu.Lock()
	if *d.DesktopSPICE == nil {
		*d.DesktopSPICE = make(map[string]DesktopSPICEProxyTarget, 64)
	}
	(*d.DesktopSPICE)[sessionID] = target
	d.DesktopSPICEMu.Unlock()
}

func (d *Deps) TakeDesktopSPICEProxyTarget(sessionID string) (DesktopSPICEProxyTarget, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return DesktopSPICEProxyTarget{}, false
	}
	d.DesktopSPICEMu.Lock()
	defer d.DesktopSPICEMu.Unlock()
	if *d.DesktopSPICE == nil {
		return DesktopSPICEProxyTarget{}, false
	}
	target, ok := (*d.DesktopSPICE)[sessionID]
	if ok {
		delete(*d.DesktopSPICE, sessionID)
	}
	return target, ok
}

func (d *Deps) ClearDesktopSPICEProxyTarget(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	d.DesktopSPICEMu.Lock()
	if *d.DesktopSPICE != nil {
		delete(*d.DesktopSPICE, sessionID)
	}
	d.DesktopSPICEMu.Unlock()
}

func (d *Deps) TerminateDesktopSession(session terminal.Session) error {
	if rawBridge, ok := d.DesktopBridges.Load(session.ID); ok {
		if bridgeState, ok := rawBridge.(*DesktopBridge); ok {
			var agentConn *agentmgr.AgentConn
			if d.AgentMgr != nil {
				agentConn, _ = d.AgentMgr.Get(session.Target)
			}
			d.FinalizeAgentDesktopSession(session.ID, bridgeState, agentConn, true, nil)
		} else {
			d.DesktopBridges.Delete(session.ID)
		}
	}

	if rawBridge, ok := d.WebRTCBridges.Load(session.ID); ok {
		if bridge, ok := rawBridge.(*WebRTCSignalingBridge); ok {
			bridge.Close()
			if d.AgentMgr != nil {
				if agentConn, ok := d.AgentMgr.Get(session.Target); ok {
					SendWebRTCStop(agentConn, session.ID)
				}
			}
		}
		d.WebRTCBridges.Delete(session.ID)
	}

	d.ClearDesktopSessionOptions(session.ID)
	d.ClearDesktopSPICEProxyTarget(session.ID)
	if err := d.TerminalStore.DeleteTerminalSession(session.ID); err != nil && !errors.Is(err, terminal.ErrSessionNotFound) {
		return err
	}
	return nil
}

// HandleDesktopSessions handles POST /desktop/sessions (create a desktop session).
func (d *Deps) HandleDesktopSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if !d.EnforceRateLimit(w, r, "desktop.session.create", 60, time.Minute) {
		return
	}

	var req struct {
		Target   string `json:"target"`
		Quality  string `json:"quality,omitempty"`
		Display  string `json:"display,omitempty"`
		Protocol string `json:"protocol,omitempty"`
		Record   bool   `json:"record,omitempty"`
	}
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	target := strings.TrimSpace(req.Target)
	if target == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "target is required")
		return
	}
	if err := d.ValidateMaxLen("target", target, d.MaxTargetLength); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if d.AssetStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "asset inventory unavailable")
		return
	}
	if _, ok, err := d.AssetStore.GetAsset(target); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to resolve desktop target")
		return
	} else if !ok {
		servicehttp.WriteError(w, http.StatusBadRequest, "target must reference a managed asset")
		return
	}

	actorID := d.PrincipalActorID(r.Context())
	checkRes := policy.Evaluate(policy.CheckRequest{
		ActorID: actorID,
		Target:  target,
		Mode:    "interactive",
		Action:  "session_start",
	}, d.PolicyState.Current())
	if !checkRes.Allowed {
		servicehttp.WriteError(w, http.StatusForbidden, checkRes.Reason)
		return
	}

	session, err := d.TerminalStore.CreateSession(terminal.CreateSessionRequest{
		ActorID: actorID,
		Target:  target,
		Mode:    "desktop",
	})
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create desktop session")
		return
	}

	quality := strings.ToLower(strings.TrimSpace(req.Quality))
	if quality == "" {
		quality = "medium"
	}
	if quality != "low" && quality != "medium" && quality != "high" {
		quality = "medium"
	}
	d.SetDesktopSessionOptions(session.ID, DesktopSessionOptions{
		Protocol: NormalizeDesktopProtocol(req.Protocol),
		Quality:  quality,
		Display:  strings.TrimSpace(req.Display),
		Record:   req.Record,
	})
	go func(sessionID string) {
		timer := time.NewTimer(15 * time.Minute)
		defer timer.Stop()
		<-timer.C
		d.ClearDesktopSessionOptions(sessionID)
		d.ClearDesktopSPICEProxyTarget(sessionID)
	}(session.ID)

	servicehttp.WriteJSON(w, http.StatusCreated, session)
}

// HandleDesktopSessionActions handles sub-routes under /desktop/sessions/{id}/...
func (d *Deps) HandleDesktopSessionActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/desktop/sessions/")
	if path == "" || path == r.URL.Path {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	parts := strings.SplitN(path, "/", 2)
	sessionID := strings.TrimSpace(parts[0])
	if sessionID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "session id required")
		return
	}

	session, ok, err := d.TerminalStore.GetSession(sessionID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "session not found")
		return
	}
	if strings.TrimSpace(session.Mode) != "desktop" {
		servicehttp.WriteError(w, http.StatusNotFound, "session not found")
		return
	}
	if !d.CanAccessOwnedSession(r, session.ActorID) {
		servicehttp.WriteError(w, http.StatusForbidden, "session access denied")
		return
	}

	if len(parts) < 2 {
		if r.Method == http.MethodDelete {
			if err := d.TerminateDesktopSession(session); err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete desktop session")
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, session)
		return
	}

	action := strings.TrimSpace(parts[1])
	switch action {
	case "stream-ticket":
		d.HandleDesktopStreamTicket(w, r, session)
	case "stream":
		d.HandleDesktopStream(w, r, session)
	case "audio":
		d.HandleDesktopAudioStream(w, r, session)
	case "spice-ticket":
		d.HandleDesktopSPICETicket(w, r, session)
	default:
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
	}
}

// HandleDesktopStreamTicket issues a one-time stream ticket for desktop WebSocket auth.
func (d *Deps) HandleDesktopStreamTicket(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.EnforceRateLimit(w, r, "desktop.stream_ticket.create", 240, time.Minute) {
		return
	}

	ticket, expiresAt, err := d.IssueStreamTicket(r.Context(), session.ID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to issue stream ticket")
		return
	}
	audioTicket := ""

	opts := d.GetDesktopSessionOptions(session.ID)
	effectiveProtocol := d.ResolveDesktopProtocol(session, r)
	if effectiveProtocol == "vnc" && d.AgentMgr != nil && d.AgentMgr.IsConnected(session.Target) {
		if strings.TrimSpace(opts.VNCPassword) == "" {
			password, err := generateSessionVNCPassword()
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to prepare desktop session auth")
				return
			}
			opts.VNCPassword = password
			d.SetDesktopSessionOptions(session.ID, opts)
		}
		audioTicket, _, err = d.IssueStreamTicket(r.Context(), session.ID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to prepare desktop audio stream")
			return
		}
	}

	values := neturl.Values{}
	values.Set("ticket", ticket)
	if opts.Quality != "" {
		values.Set("quality", opts.Quality)
	}
	if opts.Display != "" {
		values.Set("display", opts.Display)
	}
	if effectiveProtocol != "" && effectiveProtocol != "vnc" {
		values.Set("protocol", effectiveProtocol)
	}
	if opts.Record {
		values.Set("record", "1")
	}
	streamPath := fmt.Sprintf(
		"/desktop/sessions/%s/stream?%s",
		neturl.PathEscape(session.ID),
		values.Encode(),
	)
	response := map[string]any{
		"session_id":  session.ID,
		"ticket":      ticket,
		"expires_at":  expiresAt,
		"stream_path": streamPath,
		"protocol":    effectiveProtocol,
	}
	if strings.TrimSpace(opts.VNCPassword) != "" {
		response["vnc_password"] = opts.VNCPassword
	}
	if strings.TrimSpace(audioTicket) != "" {
		response["audio_stream_path"] = fmt.Sprintf(
			"/desktop/sessions/%s/audio?ticket=%s",
			neturl.PathEscape(session.ID),
			neturl.QueryEscape(audioTicket),
		)
	}

	servicehttp.WriteJSON(w, http.StatusCreated, response)
}
