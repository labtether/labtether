package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/connectors/docker"
	respkg "github.com/labtether/labtether/internal/hubapi/resources"
	"github.com/labtether/labtether/internal/persistence"
)

// Probe identities for the route access pin. The keys have the admin role so
// that the role gate does not hide the scope and device checks behind it.
const (
	routePinAnonymous  = "anonymous"
	routePinViewer     = "viewer"
	routePinOperator   = "operator"
	routePinAdmin      = "admin"
	routePinKeyNoScope = "key-no-scope"
	routePinKeyOther   = "key-other-asset"

	// routePinAsset is the device ID put in every subtree probe path.
	routePinAsset = "nas01"
	// routePinOtherAsset is the only device the restricted key may reach.
	routePinOtherAsset = "nas02"
	// routePinUselessScope is a valid scope string that no route requires,
	// because an API key must carry at least one scope.
	routePinUselessScope = "search:none"
)

var routePinPrincipals = []string{
	routePinAnonymous, routePinViewer, routePinOperator, routePinAdmin,
	routePinKeyNoScope, routePinKeyOther,
}

// routePinStreamRoutes keep a request open until the client leaves or a
// timer ends (the pprof ones sample for seconds). The probe gives them a
// context that ends after a short time, so they return.
var routePinStreamRoutes = map[string]bool{
	"/api/v2/events/stream":  true,
	"/status/aggregate/live": true,
	"/mcp":                   true,
	"/debug/pprof/profile":   true,
	"/debug/pprof/trace":     true,
}

const routePinStreamWindow = 150 * time.Millisecond

// routePinFixture is one fresh in-memory Hub with the probe identities.
type routePinFixture struct {
	sut      *apiServer
	handlers map[string]http.HandlerFunc
	creds    map[string]func(*http.Request)
}

// sandboxRoutePins keeps handler side effects inside the test: the working
// directory is a temporary folder, no tailscale binary can be found or run,
// and a hub restart request does not signal the test process.
func sandboxRoutePins(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	origLookPath, origFallback := tailscaleLookPath, tailscaleFallbackPaths
	origRestart := respkg.HubRestartSelf
	t.Cleanup(func() {
		tailscaleLookPath, tailscaleFallbackPaths = origLookPath, origFallback
		respkg.HubRestartSelf = origRestart
	})
	tailscaleLookPath = func(string) (string, error) { return "", errors.New("tailscale disabled in wiring pin") }
	tailscaleFallbackPaths = func() []string { return nil }
	respkg.HubRestartSelf = func() error { return nil }
}

func newRoutePinFixture(t *testing.T) *routePinFixture {
	t.Helper()
	sut := newTestAPIServer(t)
	sut.dataDir = t.TempDir()
	// Stores the plain test Hub lacks, so handlers that refuse with 503 when a
	// store is missing get as far as their own scope checks.
	sut.edgeStore = newMockEdgeStore()
	sut.dependencyStore = routePinDependencyStore{}
	sut.dockerCoordinator = docker.NewCoordinator(nil)
	f := &routePinFixture{sut: sut, creds: map[string]func(*http.Request){}}
	f.creds[routePinAnonymous] = func(*http.Request) {}
	for _, role := range []string{auth.RoleViewer, auth.RoleOperator, auth.RoleAdmin} {
		token := createRoleSessionToken(t, sut, "pin-"+role, role)
		f.creds[role] = func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
		}
	}
	noScope := createLegacyRouteAPIKeyWithRole(t, sut, auth.RoleAdmin, []string{routePinUselessScope}, nil)
	f.creds[routePinKeyNoScope] = routePinBearer(noScope)
	other := createLegacyRouteAPIKeyWithRole(t, sut, auth.RoleAdmin, []string{"*"}, []string{routePinOtherAsset})
	f.creds[routePinKeyOther] = routePinBearer(other)
	f.handlers = sut.buildHTTPHandlers(nil, nil, nil)
	return f
}

// routePinDependencyStore is present but empty-handed: any call into it
// panics, which the probe records. It only lets handlers pass their
// "store missing" check.
type routePinDependencyStore struct{ persistence.DependencyStore }

func routePinBearer(key string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+key) }
}

// routePinProbePath is the request path used for a route: the route itself,
// or for a subtree route ("/x/") the route plus one device ID.
func routePinProbePath(route string) string {
	if route != "/" && strings.HasSuffix(route, "/") {
		return route + routePinAsset
	}
	return route
}

// call sends one bare request (no body) as the given identity and returns
// the outcome as one short, stable string.
func (f *routePinFixture) call(t *testing.T, route, method, principal string) string {
	t.Helper()
	handler := f.handlers[route]
	if handler == nil {
		return "missing route"
	}
	req := httptest.NewRequest(method, routePinProbePath(route), nil)
	req.TLS = &tls.ConnectionState{}
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
	f.creds[principal](req)
	if routePinStreamRoutes[route] {
		ctx, cancel := context.WithTimeout(req.Context(), routePinStreamWindow)
		defer cancel()
		req = req.WithContext(ctx)
	}
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	var panicked any
	go func() {
		defer close(done)
		// The test Hub leaves some production fields nil. A handler that
		// reaches one panics; production's recovery layer would answer 500.
		defer func() { panicked = recover() }()
		handler(rec, req)
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatalf("%s %s as %s did not return; add it to routePinStreamRoutes or the exceptions", method, route, principal)
	}
	if panicked != nil {
		return fmt.Sprintf("panic %q", clipRoutePinText(fmt.Sprint(panicked)))
	}
	return routePinOutcome(rec)
}

// routePinOutcome keeps the status code and, for answers that are not 2xx,
// the error words. A scope or device refusal is shortened to what it names.
func routePinOutcome(rec *httptest.ResponseRecorder) string {
	code := rec.Code
	if code >= 200 && code < 300 {
		return fmt.Sprintf("%d", code)
	}
	body := strings.TrimSpace(rec.Body.String())
	var envelope map[string]any
	if json.Unmarshal([]byte(body), &envelope) == nil {
		errCode, _ := envelope["error"].(string)
		message, _ := envelope["message"].(string)
		switch {
		case errCode == "insufficient_scope":
			return fmt.Sprintf("%d scope=%s", code, strings.TrimPrefix(message, "api key lacks required scope: "))
		case errCode == "asset_forbidden":
			return fmt.Sprintf("%d asset-denied", code)
		case message != "":
			return fmt.Sprintf("%d %s %q", code, errCode, clipRoutePinText(message))
		case errCode != "":
			return fmt.Sprintf("%d %q", code, clipRoutePinText(errCode))
		}
	}
	if body == "" {
		return fmt.Sprintf("%d", code)
	}
	return fmt.Sprintf("%d text %q", code, clipRoutePinText(strings.SplitN(body, "\n", 2)[0]))
}

func clipRoutePinText(text string) string {
	text = strings.TrimSpace(text)
	if len(text) > 90 {
		return text[:90] + "..."
	}
	return text
}
