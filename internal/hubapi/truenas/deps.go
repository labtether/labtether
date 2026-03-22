package truenas

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	tnconnector "github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/secrets"
)

// Deps holds all dependencies required by the truenas handler package.
// Store interfaces are embedded directly; cross-cutting concerns that live
// in other cmd/labtether subsystems are injected as function fields.
type Deps struct {
	// Store interfaces
	AssetStore        persistence.AssetStore
	HubCollectorStore persistence.HubCollectorStore
	CredentialStore   persistence.CredentialStore
	SecretsManager    *secrets.Manager
	LogStore          persistence.LogStore

	// Runtime client cache (owned by this package after extraction).
	TruenasCacheMu sync.RWMutex
	TruenasCache   map[string]*CachedTrueNASRuntime

	// Read-data caches (SMART, filesystem).
	TruenasReadCacheMu sync.RWMutex
	TruenasSmartCache  map[string]TrueNASAssetSMARTResponse
	TruenasFSCache     map[string]TrueNASFilesystemResponse

	// Subscription worker state.
	TruenasSubMu sync.Mutex
	TruenasSubs  map[string]TruenasSubscriptionHandle

	// Auth middleware injected from cmd/labtether.
	RequireAdminAuth func(w http.ResponseWriter, r *http.Request) bool

	// WrapAuth / WrapAdmin for route registration.
	WrapAuth  func(http.HandlerFunc) http.HandlerFunc
	WrapAdmin func(http.HandlerFunc) http.HandlerFunc

	// CheckSameOrigin is injected from cmd/labtether for WebSocket origin validation.
	CheckSameOrigin func(r *http.Request) bool

	// Logging helpers injected from cmd/labtether.
	AppendConnectorLogEvent       func(assetID, source, level, message string, fields map[string]string, at time.Time)
	AppendConnectorLogEventWithID func(eventID, assetID, source, level, message string, fields map[string]string, at time.Time)

	// WebSocket keepalive helpers for terminal proxy.
	StartBrowserWebSocketKeepalive    func(wsConn *websocket.Conn, writeMu *sync.Mutex, streamLabel string) func()
	TouchBrowserWebSocketReadDeadline func(wsConn *websocket.Conn) error
}

// TruenasRuntime holds a resolved TrueNAS client and its metadata.
type TruenasRuntime struct {
	Client      *tnconnector.Client
	CollectorID string
	BaseURL     string
	APIKey      string // #nosec G117 -- Runtime connector credential, not a hardcoded secret.
	SkipVerify  bool
	Timeout     time.Duration
	ConfigKey   string
}

// CachedTrueNASRuntime wraps a runtime with its config key for cache invalidation.
type CachedTrueNASRuntime struct {
	Runtime   *TruenasRuntime
	ConfigKey string
}

// TruenasSubscriptionHandle holds the config key and cancel function for a subscription worker.
type TruenasSubscriptionHandle struct {
	ConfigKey string
	Cancel    context.CancelFunc
}

// TruenasShellTarget holds the resolved connection info for a TrueNAS shell session.
type TruenasShellTarget struct {
	BaseURL    string
	APIKey     string // #nosec G117 -- Runtime connector credential, not a hardcoded secret.
	SkipVerify bool
	Timeout    time.Duration
	Options    map[string]any // vm_id, app_name, container_id
}

// RegisterRoutes registers all TrueNAS API routes on the given mux.
func RegisterRoutes(mux *http.ServeMux, d *Deps) {
	mux.HandleFunc("/truenas/assets/", d.WrapAuth(d.HandleTrueNASAssets))
}

// SelectCollectorForTrueNASRuntime is the exported wrapper for selectCollectorForTrueNASRuntime.
func SelectCollectorForTrueNASRuntime(collectors []hubcollector.Collector, collectorID string) *hubcollector.Collector {
	return selectCollectorForTrueNASRuntime(collectors, collectorID)
}

// QueryEventsForAsset queries log events for a TrueNAS asset.
func (d *Deps) QueryEventsForAsset(req logs.QueryRequest) ([]logs.Event, error) {
	if d.LogStore == nil {
		return nil, nil
	}
	return d.LogStore.QueryEvents(req)
}
