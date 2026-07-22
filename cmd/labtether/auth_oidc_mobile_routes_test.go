package main

import "testing"

func TestMobileOIDCRoutesAreRegistered(t *testing.T) {
	handlers := (&apiServer{}).buildHTTPHandlers(nil, nil, nil)
	for _, path := range []string{
		"/auth/oidc/mobile/start",
		"/auth/oidc/mobile/callback",
	} {
		if handlers[path] == nil {
			t.Fatalf("mobile OIDC route %q is not registered", path)
		}
	}
}
