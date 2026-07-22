package main

import "testing"

func TestBuildHTTPHandlersRegistersPortainerEndpoints(t *testing.T) {
	handlers := (&apiServer{}).buildHTTPHandlers(nil, nil, nil)
	if handlers["/portainer/endpoints"] == nil {
		t.Fatal("/portainer/endpoints route is not registered")
	}
}
