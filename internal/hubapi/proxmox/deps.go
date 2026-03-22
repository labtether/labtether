package proxmox

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/secrets"
)

// DesktopSPICEProxyTarget holds the parameters needed to connect a browser
// to a Proxmox SPICE session.
type DesktopSPICEProxyTarget struct {
	Host       string
	TLSPort    int
	Password   string // #nosec G117 -- Runtime connector credential, not a hardcoded secret.
	Type       string
	CA         string
	Proxy      string
	SkipVerify bool
}

// Deps holds all dependencies required by the proxmox handler package.
// Store interfaces are embedded directly; cross-cutting concerns that live
// in other cmd/labtether subsystems are injected as function fields.
type Deps struct {
	// Store interfaces
	AssetStore        persistence.AssetStore
	HubCollectorStore persistence.HubCollectorStore
	CredentialStore   persistence.CredentialStore
	SecretsManager    *secrets.Manager
	TelemetryStore    persistence.TelemetryStore
	ConnectorRegistry *connectorsdk.Registry

	// Proxmox client cache (owned by this package after extraction).
	ProxmoxCacheMu sync.RWMutex
	ProxmoxCache   map[string]*CachedProxmoxRuntime

	// Auth middleware injected from cmd/labtether.
	RequireAdminAuth func(w http.ResponseWriter, r *http.Request) bool
	EnforceRateLimit func(w http.ResponseWriter, r *http.Request, bucket string, limit int, window time.Duration) bool

	// Stream ticket system.
	IssueStreamTicket func(ctx context.Context, sessionID string) (string, time.Time, error)

	// Desktop SPICE proxy target store.
	SetDesktopSPICEProxyTarget  func(sessionID string, target DesktopSPICEProxyTarget)
	TakeDesktopSPICEProxyTarget func(sessionID string) (DesktopSPICEProxyTarget, bool)

	// Command execution (for action runtime).
	ExecuteCommandAction     func(job actions.Job) actions.Result
	ExecuteActionInProcessFn func(job actions.Job, registry *connectorsdk.Registry) actions.Result

	// Terminal/desktop stream infrastructure injected from cmd/labtether.
	TerminalWebSocketUpgrader *websocket.Upgrader
	MaxDesktopInputReadBytes  int64

	// WrapAuth / WrapAdmin for route registration.
	WrapAuth  func(http.HandlerFunc) http.HandlerFunc
	WrapAdmin func(http.HandlerFunc) http.HandlerFunc
}

// RegisterRoutes registers all Proxmox API routes on the given mux.
func RegisterRoutes(mux *http.ServeMux, d *Deps) {
	mux.HandleFunc("/proxmox/assets/", d.WrapAuth(d.HandleProxmoxAssets))
	mux.HandleFunc("/proxmox/tasks/", d.WrapAuth(d.HandleProxmoxTaskRoutes))
	mux.HandleFunc("/proxmox/cluster/status", d.WrapAuth(d.HandleProxmoxClusterStatus))
	mux.HandleFunc("/proxmox/cluster/resources", d.WrapAuth(d.HandleProxmoxClusterResources))
	mux.HandleFunc("/proxmox/nodes/", d.WrapAuth(d.HandleProxmoxNodeRoutes))
	mux.HandleFunc("/proxmox/ceph/", d.WrapAuth(d.HandleProxmoxCeph))
	mux.HandleFunc("/proxmox/ceph/status", d.WrapAuth(d.HandleProxmoxCephStatus))
}
