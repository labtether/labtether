package main

import (
	"fmt"
	"strings"
	"testing"
)

// servicehttpBuiltinRoutes are registered by servicehttp.Run itself, beside
// the map from buildHTTPHandlers. This list is written here by hand: the test
// cannot see Run's mux without starting a listener. It only proves that the
// map does not also claim these paths (Run would panic at startup).
var servicehttpBuiltinRoutes = []string{"/healthz", "/readyz", "/version"}

// TestWiringPinRouteList pins the exact set of route keys, its size, and the
// routes DEV_MODE adds. Risk covered: a C2 move that drops, renames or adds a
// route (including a lost trailing slash, which changes subtree matching).
// It does not see what each route does; TestWiringPinRouteAccess does.
//
// Regenerate (then review the diff line by line):
//
//	LABTETHER_UPDATE_WIRING_PINS=1 go test -count=1 -run 'TestWiringPin' ./cmd/labtether
func TestWiringPinRouteList(t *testing.T) {
	t.Setenv("DEV_MODE", "")
	production := sortedRouteKeys(newTestAPIServer(t).buildHTTPHandlers(nil, nil, nil))
	t.Setenv("DEV_MODE", "true")
	withDev := sortedRouteKeys(newTestAPIServer(t).buildHTTPHandlers(nil, nil, nil))

	inProduction := map[string]bool{}
	for _, route := range production {
		inProduction[route] = true
	}
	var devOnly []string
	for _, route := range withDev {
		if !inProduction[route] {
			devOnly = append(devOnly, route)
		}
	}
	if len(withDev) != len(production)+len(devOnly) {
		t.Fatalf("DEV_MODE removed routes: production=%d dev=%d dev-only=%d", len(production), len(withDev), len(devOnly))
	}
	for _, route := range servicehttpBuiltinRoutes {
		if inProduction[route] {
			t.Fatalf("buildHTTPHandlers registers %s, which servicehttp.Run already serves", route)
		}
	}

	var b strings.Builder
	b.WriteString("# Route list pin: keys of buildHTTPHandlers, sorted.\n")
	b.WriteString("# Regenerate: " + wiringPinUpdateCommand + "\n")
	writeRouteListBlock(&b, "production routes", production)
	writeRouteListBlock(&b, "added when DEV_MODE=true", devOnly)
	writeRouteListBlock(&b, "served by servicehttp.Run (hand-written list, not read from code)", servicehttpBuiltinRoutes)
	fmt.Fprintf(&b, "\n== total served with DEV_MODE off\ncount %d\n", len(production)+len(servicehttpBuiltinRoutes))
	compareWiringPin(t, "routes.txt", []byte(b.String()))
}

func writeRouteListBlock(b *strings.Builder, title string, routes []string) {
	fmt.Fprintf(b, "\n== %s\ncount %d\n", title, len(routes))
	for _, route := range routes {
		b.WriteString(route + "\n")
	}
}
