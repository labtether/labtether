package proxmox

import (
	"crypto/cipher"
	// #nosec G502 -- Proxmox VNC auth follows the RFB protocol which mandates DES challenge encryption.
	"crypto/des"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/connectors/proxmox"
)

var ProxmoxTerminalKeepaliveInterval = 30 * time.Second
var ProxmoxStreamHooksMu sync.RWMutex

var ProxmoxWSWriteMessage = func(conn *websocket.Conn, messageType int, data []byte) error {
	return conn.WriteMessage(messageType, data)
}

var ProxmoxWSReadMessage = func(conn *websocket.Conn) (int, []byte, error) {
	return conn.ReadMessage()
}

var ProxmoxWSSetReadDeadline = func(conn *websocket.Conn, deadline time.Time) error {
	return conn.SetReadDeadline(deadline)
}

var ProxmoxDESNewCipher = des.NewCipher

func proxmoxCurrentKeepaliveInterval() time.Duration {
	ProxmoxStreamHooksMu.RLock()
	defer ProxmoxStreamHooksMu.RUnlock()
	return ProxmoxTerminalKeepaliveInterval
}

func proxmoxCallWriteMessage(conn *websocket.Conn, messageType int, data []byte) error {
	ProxmoxStreamHooksMu.RLock()
	hook := ProxmoxWSWriteMessage
	ProxmoxStreamHooksMu.RUnlock()
	return hook(conn, messageType, data)
}

func proxmoxCallReadMessage(conn *websocket.Conn) (int, []byte, error) {
	ProxmoxStreamHooksMu.RLock()
	hook := ProxmoxWSReadMessage
	ProxmoxStreamHooksMu.RUnlock()
	return hook(conn)
}

func proxmoxCallSetReadDeadline(conn *websocket.Conn, deadline time.Time) error {
	ProxmoxStreamHooksMu.RLock()
	hook := ProxmoxWSSetReadDeadline
	ProxmoxStreamHooksMu.RUnlock()
	return hook(conn, deadline)
}

func proxmoxCallDESNewCipher(key []byte) (cipher.Block, error) {
	ProxmoxStreamHooksMu.RLock()
	hook := ProxmoxDESNewCipher
	ProxmoxStreamHooksMu.RUnlock()
	return hook(key)
}

type ProxmoxRuntime struct {
	client      *proxmox.Client
	authMode    proxmox.AuthMode
	tokenID     string
	tokenSecret string
	skipVerify  bool
	caPEM       string
	collectorID string
}

// Client returns the underlying proxmox API client.
func (r *ProxmoxRuntime) Client() *proxmox.Client { return r.client }

// NewProxmoxRuntime creates a ProxmoxRuntime for use in tests and external construction.
func NewProxmoxRuntime(client *proxmox.Client) *ProxmoxRuntime {
	return &ProxmoxRuntime{client: client}
}

// NewProxmoxRuntimeWithCollector creates a ProxmoxRuntime with a collector ID.
func NewProxmoxRuntimeWithCollector(client *proxmox.Client, collectorID string) *ProxmoxRuntime {
	return &ProxmoxRuntime{client: client, collectorID: collectorID}
}

// CollectorID returns the collector ID for this runtime.
func (r *ProxmoxRuntime) CollectorID() string { return r.collectorID }

// SkipVerify returns the skip-verify setting for this runtime.
func (r *ProxmoxRuntime) SkipVerify() bool { return r.skipVerify }

// AuthMode returns the authentication mode for this runtime.
func (r *ProxmoxRuntime) AuthMode() proxmox.AuthMode { return r.authMode }

// ProxmoxRuntimeOpts holds options for constructing a ProxmoxRuntime.
type ProxmoxRuntimeOpts struct {
	Client      *proxmox.Client
	AuthMode    proxmox.AuthMode
	TokenID     string
	TokenSecret string
	SkipVerify  bool
	CAPEM       string
	CollectorID string
}

// NewProxmoxRuntimeOpts creates a ProxmoxRuntime from options.
func NewProxmoxRuntimeOpts(opts ProxmoxRuntimeOpts) *ProxmoxRuntime {
	return &ProxmoxRuntime{
		client:      opts.Client,
		authMode:    opts.AuthMode,
		tokenID:     opts.TokenID,
		tokenSecret: opts.TokenSecret,
		skipVerify:  opts.SkipVerify,
		caPEM:       opts.CAPEM,
		collectorID: opts.CollectorID,
	}
}

type CachedProxmoxRuntime struct {
	runtime   *ProxmoxRuntime
	configKey string
}

type ProxmoxSessionTarget struct {
	Kind        string
	Node        string
	VMID        string
	StorageName string
	CollectorID string
}
