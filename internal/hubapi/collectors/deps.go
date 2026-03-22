package collectors

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/connectors/webservice"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/secrets"

	"golang.org/x/crypto/ssh"
)

// Deps holds all dependencies required by the collectors handler package.
// Store interfaces are embedded directly; cross-cutting concerns that live
// in other cmd/labtether subsystems are injected as function fields.
type Deps struct {
	// Store interfaces
	AssetStore        persistence.AssetStore
	HubCollectorStore persistence.HubCollectorStore
	CredentialStore   persistence.CredentialStore
	SecretsManager    *secrets.Manager
	TelemetryStore    persistence.TelemetryStore
	LogStore          persistence.LogStore
	DependencyStore   persistence.DependencyStore
	ConnectorRegistry *connectorsdk.Registry
	RuntimeStore      persistence.RuntimeSettingsStore
	DB                *persistence.PostgresStore

	// Agent manager for web service report dispatch.
	AgentMgr *agentmgr.AgentManager

	// Web service coordinator.
	WebServiceCoordinator *webservice.Coordinator

	// Concurrency control for collector runs.
	CollectorDispatchSem chan struct{}
	CollectorRunState    sync.Map // map[collectorID]*atomic.Bool

	// Web service URL grouping cache (owned by this package).
	WebServiceURLGroupingCfgMu  sync.RWMutex
	WebServiceURLGroupingCfg    WebServiceURLGroupingConfig
	WebServiceURLGroupingCfgAt  time.Time
	WebServiceURLGroupingCfgTTL time.Duration
	URLGroupingSuggestions      []WebServiceGroupingSuggestion
	URLGroupingSuggestionsMu    sync.Mutex

	// Link suggestion scan state.
	LinkSuggestionScanMu          sync.Mutex
	LinkSuggestionScanLastStarted time.Time
	LinkSuggestionScanRunning     atomic.Bool

	// Auth middleware injected from cmd/labtether.
	EnforceRateLimit func(w http.ResponseWriter, r *http.Request, bucket string, limit int, window time.Duration) bool

	// Cross-cutting methods injected from cmd/labtether.
	ProcessHeartbeatRequest           func(req assets.HeartbeatRequest) (*assets.Asset, error)
	PersistCanonicalConnectorSnapshot func(connectorID, collectorID, displayName, parentAssetID string, connector connectorsdk.Connector, assets []connectorsdk.Asset)
	DetectLinkSuggestions             func() error
	ExecuteProxmoxActionDirect        func(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error)
	// Connector runtime loaders injected from cmd/labtether.
	LoadProxmoxRuntime              func(collectorID string) (*proxmox.Client, error)
	LoadTrueNASRuntime              func(collectorID string) (*truenas.Client, error)
	EnsureTrueNASSubscriptionWorker func(ctx context.Context, collectorID string, client *truenas.Client)

	// SSH host key callback builder (from cmd/labtether).
	BuildKnownHostsHostKeyCallback func() (ssh.HostKeyCallback, error)

	// WrapAuth / WrapAdmin for route registration.
	WrapAuth  func(http.HandlerFunc) http.HandlerFunc
	WrapAdmin func(http.HandlerFunc) http.HandlerFunc
}

// RegisterRoutes registers all collector-related HTTP routes on the given mux.
func RegisterRoutes(mux *http.ServeMux, d *Deps) {
	mux.HandleFunc("/hub-collectors", d.WrapAuth(d.HandleHubCollectors))
	mux.HandleFunc("/hub-collectors/", d.WrapAuth(d.HandleHubCollectorActions))
	mux.HandleFunc("/connectors", d.WrapAuth(d.HandleListConnectors))
	mux.HandleFunc("/connectors/", d.WrapAuth(d.HandleConnectorActions))
	mux.HandleFunc("/api/v1/services/web", d.WrapAuth(d.HandleWebServices))
	mux.HandleFunc("/api/v1/services/web/compat", d.WrapAuth(d.HandleWebServiceCompat))
	mux.HandleFunc("/api/v1/services/web/categories", d.WrapAuth(d.HandleWebServiceCategories))
	mux.HandleFunc("/api/v1/services/web/sync", d.WrapAuth(d.HandleWebServiceSync))
	mux.HandleFunc("/api/v1/services/web/manual", d.WrapAuth(d.HandleWebServiceManual))
	mux.HandleFunc("/api/v1/services/web/manual/", d.WrapAuth(d.HandleWebServiceManualActions))
	mux.HandleFunc("/api/v1/services/web/overrides", d.WrapAuth(d.HandleWebServiceOverrides))
	mux.HandleFunc("/api/v1/services/web/icon-library", d.WrapAuth(d.HandleWebServiceIconLibrary))
	mux.HandleFunc("/api/v1/services/web/alt-urls/", d.WrapAuth(d.HandleWebServiceAltURLs))
	mux.HandleFunc("/api/v1/services/web/never-group-rules", d.WrapAuth(d.HandleWebServiceNeverGroupRules))
	mux.HandleFunc("/api/v1/services/web/grouping-settings", d.WrapAuth(d.HandleWebServiceGroupingSettings))
	mux.HandleFunc("/api/v1/services/web/grouping-suggestions/", d.WrapAuth(d.HandleWebServiceGroupingSuggestionResponse))
}

// RegisterWSHandlers registers WebSocket message handlers for collector-related
// agent messages.
func RegisterWSHandlers(router map[string]func(*agentmgr.AgentConn, agentmgr.Message), d *Deps) {
	router[agentmgr.MsgWebServiceReport] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentWebServiceReport(conn, msg)
	}
}
