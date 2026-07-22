package main

import "testing"

func TestQuickSessionRouteIsRegistered(t *testing.T) {
	handlers := (&apiServer{}).buildHTTPHandlers(nil, nil, nil)
	if handlers["/terminal/quick-session"] == nil {
		t.Fatal("expected /terminal/quick-session to be registered")
	}
}
