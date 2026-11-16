package main

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"
)

// TestWiringPinRouteAccess pins who may call every route, without reading
// handler code. Each route gets a fresh in-memory Hub. It is called with no
// body as six identities (no credentials; viewer, operator and admin
// sessions; an admin-role API key whose only scope grants nothing; an
// admin-role API key with every scope but limited to another device), with
// GET and POST, and with PUT, PATCH and DELETE when an admin gets 405 for
// both GET and POST. The answer shows the wrapper (401 without credentials),
// the role floor (403 for viewer or operator), the scope the route or the v2
// handler inside it checks (scope=...), and device checks (asset-denied).
//
// Risk covered: a C2 move that drops a route, swaps withAdminAuth for
// withAuth, loses a requireScope, or drops a scope check inside a v2 handler.
// It cannot see what a handler does with a valid body, or checks that run
// only after body parsing. Calls in routePinExceptions are not made.
//
// Regenerate (then review the diff line by line):
//
//	LABTETHER_UPDATE_WIRING_PINS=1 go test -count=1 -run 'TestWiringPin' ./cmd/labtether
func TestWiringPinRouteAccess(t *testing.T) {
	t.Setenv("DEV_MODE", "")
	sandboxRoutePins(t)
	routes := sortedRouteKeys(newRoutePinFixture(t).handlers)
	compareWiringPin(t, "route_access.txt", []byte(routePinReport(t, routes, routeAccessHeader)))
}

// TestWiringPinDevRouteAccess pins the routes only DEV_MODE adds.
func TestWiringPinDevRouteAccess(t *testing.T) {
	t.Setenv("DEV_MODE", "true")
	sandboxRoutePins(t)
	var routes []string
	for _, route := range sortedRouteKeys(newRoutePinFixture(t).handlers) {
		if strings.HasPrefix(route, "/debug/") {
			routes = append(routes, route)
		}
	}
	compareWiringPin(t, "route_access_dev.txt", []byte(routePinReport(t, routes, devRouteAccessHeader)))
}

const routeAccessHeader = `# Route access pin: every route in buildHTTPHandlers (DEV_MODE off).
# Each block: route, the request path used, then one line per method and identity.
# Identities: anonymous; viewer, operator, admin sessions; key-no-scope (admin role,
# only scope "search:none"); key-other-asset (admin role, scope "*", device nas02 only).
# Subtree routes are called with device nas01 appended. Requests have no body.
# "skipped" lines are calls not made; the reason is in routePinExceptions.
# Regenerate: LABTETHER_UPDATE_WIRING_PINS=1 go test -count=1 -run 'TestWiringPin' ./cmd/labtether
`

const devRouteAccessHeader = `# Route access pin: routes added only when DEV_MODE=true.
# Same identities and format as route_access.txt.
`

// routePinExceptions lists calls the pin does not make because the handler
// would act outside the in-memory test stores. Keyed by route, then
// "METHOD identity".
var routePinExceptions = map[string]map[string]string{
	"/settings/restart": {
		"POST admin": "accepted restart signals the process 300 ms later",
	},
}

var routePinExtraMethods = []string{http.MethodPut, http.MethodPatch, http.MethodDelete}

func sortedRouteKeys(handlers map[string]http.HandlerFunc) []string {
	routes := make([]string, 0, len(handlers))
	for route := range handlers {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	return routes
}

func routePinReport(t *testing.T, routes []string, header string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString(header)
	for _, route := range routes {
		started := time.Now()
		b.WriteString("\n")
		b.WriteString(routePinBlock(t, route))
		if elapsed := time.Since(started); elapsed > 2*time.Second {
			t.Logf("route %s took %s", route, elapsed.Round(time.Millisecond))
		}
	}
	return b.String()
}

// routePinBlock probes one route on its own fresh Hub. GET calls run before
// POST calls, and identities run in a fixed order, so a call that changes
// in-memory state (a logout, a reset) affects later lines the same way on
// every run.
func routePinBlock(t *testing.T, route string) string {
	t.Helper()
	f := newRoutePinFixture(t)
	var b strings.Builder
	fmt.Fprintf(&b, "== %s (probe %s)\n", route, routePinProbePath(route))
	results := map[string]map[string]string{}
	run := func(method, principal string) string {
		if results[method] == nil {
			results[method] = map[string]string{}
		}
		if _, done := results[method][principal]; !done {
			if reason, skip := routePinExceptions[route][method+" "+principal]; skip {
				results[method][principal] = "skipped: " + reason
			} else {
				results[method][principal] = f.call(t, route, method, principal)
			}
		}
		return results[method][principal]
	}
	methods := []string{http.MethodGet, http.MethodPost}
	for _, method := range methods {
		for _, principal := range routePinPrincipals {
			run(method, principal)
		}
	}
	if strings.HasPrefix(results[http.MethodGet][routePinAdmin], "405") &&
		strings.HasPrefix(results[http.MethodPost][routePinAdmin], "405") {
		for _, method := range routePinExtraMethods {
			if strings.HasPrefix(run(method, routePinAdmin), "405") {
				continue
			}
			methods = append(methods, method)
			for _, principal := range routePinPrincipals {
				run(method, principal)
			}
		}
	}
	for _, method := range methods {
		for _, principal := range routePinPrincipals {
			fmt.Fprintf(&b, "%-6s %-15s %s\n", method, principal, results[method][principal])
		}
	}
	return b.String()
}
