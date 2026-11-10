package main

import (
	"net/http"
	"testing"
)

// The icon library is shared settings: adding, renaming or removing an icon
// needs web-services:write, as every other Services change does.
func TestServiceIconLibraryChangesNeedWriteScope(t *testing.T) {
	sut := newTestAPIServer(t)
	handlers := sut.buildHTTPHandlers(nil, nil, nil)
	readKey := createLegacyRouteAPIKey(t, sut, []string{"web-services:read"}, nil)
	const path = "/api/v1/services/web/icon-library"
	body := `{"name":"nas","data_url":"data:image/svg+xml;base64,PHN2Zy8+"}`
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if rec := invokeLegacyRoute(t, handlers[path], method, path, readKey, body); rec.Code != http.StatusForbidden {
			t.Fatalf("%s with web-services:read: got %d, want 403: %s", method, rec.Code, rec.Body.String())
		}
	}
	if rec := invokeLegacyRoute(t, handlers[path], http.MethodGet, path, readKey, ""); rec.Code == http.StatusForbidden {
		t.Fatalf("GET with web-services:read was refused: %s", rec.Body.String())
	}
}
