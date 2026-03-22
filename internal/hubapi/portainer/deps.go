package portainer

import (
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/secrets"
)

// Deps holds all dependencies required by the portainer handler package.
type Deps struct {
	// Store interfaces
	AssetStore        persistence.AssetStore
	HubCollectorStore persistence.HubCollectorStore
	CredentialStore   persistence.CredentialStore
	SecretsManager    *secrets.Manager

	// Runtime client cache.
	PortainerCacheMu sync.RWMutex
	PortainerCache   map[string]*CachedPortainerRuntime

	// Auth middleware injected from cmd/labtether.
	RequireAdminAuth func(w http.ResponseWriter, r *http.Request) bool

	// WrapAuth / WrapAdmin for route registration.
	WrapAuth  func(http.HandlerFunc) http.HandlerFunc
	WrapAdmin func(http.HandlerFunc) http.HandlerFunc

	// CheckSameOrigin for WebSocket origin validation.
	CheckSameOrigin func(r *http.Request) bool

	// Terminal WebSocket upgrader from cmd/labtether.
	TerminalWebSocketUpgrader *websocket.Upgrader

	// WebSocket keepalive helpers for exec proxy.
	StartBrowserWebSocketKeepalive    func(wsConn *websocket.Conn, writeMu *sync.Mutex, streamLabel string) func()
	TouchBrowserWebSocketReadDeadline func(wsConn *websocket.Conn) error
}

// RegisterRoutes registers all Portainer API routes on the given mux.
func RegisterRoutes(mux *http.ServeMux, d *Deps) {
	mux.HandleFunc("/portainer/endpoints", d.WrapAuth(d.HandlePortainerEndpoints))
	mux.HandleFunc("/portainer/assets/", d.WrapAuth(d.HandlePortainerAssets))
}

// DedupeNonEmptyWarnings returns a deduplicated list of non-empty warnings.
func DedupeNonEmptyWarnings(warnings []string) []string {
	if len(warnings) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(warnings))
	out := make([]string, 0, len(warnings))
	for _, w := range warnings {
		trimmed := w
		if trimmed == "" {
			continue
		}
		if seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// AppendWarning adds a warning to the list if non-empty and not already present.
func AppendWarning(existing []string, warning string) []string {
	if warning == "" {
		return existing
	}
	for _, w := range existing {
		if w == warning {
			return existing
		}
	}
	return append(existing, warning)
}
