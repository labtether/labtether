package desktop

import (
	cryptorand "crypto/rand"
	"strings"
)

// DesktopSessionOptions holds per-session desktop configuration.
type DesktopSessionOptions struct {
	Protocol                   string
	Quality                    string
	Display                    string
	Record                     bool
	VNCPassword                string
	FallbackReason             string
	Direct                     bool
	DirectHost                 string
	DirectPort                 int
	DirectUsername             string
	DirectPassword             string // #nosec G117 -- Ephemeral, in-memory-only RDP/SPICE session credential.
	VNCAllowInsecureTransport  bool
	RDPIgnoreCertificate       bool
	RDPAllowLegacySecurity     bool
	RDPCertificateFingerprints string
	SPICESecurityMode          string
	SPICECAPEM                 string
}

// DesktopSPICEProxyTarget holds SPICE proxy connection details for a session.
type DesktopSPICEProxyTarget struct {
	Host        string
	TLSPort     int
	Password    string // #nosec G117 -- Session credential is generated or supplied at runtime, not hardcoded.
	Type        string
	CA          string
	Proxy       string
	HostSubject string
	SkipVerify  bool
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
	vncPasswordAlphabet      = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789" // gitleaks:allow -- public character alphabet, not a credential
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

// agentStartsVNCWithSessionPassword reports whether the device's agent starts
// its own VNC server with the session password the Hub made. The Linux agent
// does (x11vnc). On macOS and Windows the agent only tunnels to
// the VNC server the person runs, which has its own login, so the session
// password must not be offered to the viewer: sending it was a failed login
// on that server ("authentication rejected") and the person was never asked.
// The agent does not say which it did, so this goes by the platform it
// connected with. A platform the Hub does not know keeps the password.
func (d *Deps) agentStartsVNCWithSessionPassword(assetID string) bool {
	if d == nil || d.AgentMgr == nil {
		return true
	}
	conn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(conn.Platform)) {
	case "darwin", "mac", "macos", "windows", "win32":
		return false
	}
	return true
}

func (d *Deps) SetDesktopSessionOptions(sessionID string, opts DesktopSessionOptions) {
	sessionID = strings.TrimSpace(sessionID)
	if d == nil || d.DesktopSessionMu == nil || d.DesktopSessionOpts == nil || sessionID == "" {
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
	if d == nil || d.DesktopSessionMu == nil || d.DesktopSessionOpts == nil {
		return DesktopSessionOptions{}
	}
	d.DesktopSessionMu.RLock()
	defer d.DesktopSessionMu.RUnlock()
	if *d.DesktopSessionOpts == nil {
		return DesktopSessionOptions{}
	}
	return (*d.DesktopSessionOpts)[strings.TrimSpace(sessionID)]
}

func (d *Deps) ClearDesktopSessionOptions(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if d == nil || d.DesktopSessionMu == nil || d.DesktopSessionOpts == nil || sessionID == "" {
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
