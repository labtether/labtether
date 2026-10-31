package resources

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/persistence"
)

func intPtr(value int) *int { return &value }

// WebDAV connections saved before 1.7.1 used plain HTTP on non-443 ports.
// 1.7.1 forces HTTPS by default; those connections must explain why they
// stopped working instead of failing silently or with a TLS error.
func TestLegacyPlainWebDAVConnectionsGetExplicitUpgradeNotice(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "")
	cases := []struct {
		name   string
		fc     persistence.FileConnection
		legacy bool
	}{
		{"pre-1.7 plain port", persistence.FileConnection{Protocol: "webdav", Port: intPtr(8080), ExtraConfig: map[string]any{}}, true},
		{"pre-1.7 default port was already https", persistence.FileConnection{Protocol: "webdav", ExtraConfig: map[string]any{}}, false},
		{"explicit plain without opt-in", persistence.FileConnection{Protocol: "webdav", Port: intPtr(80), ExtraConfig: map[string]any{"webdav_tls": false}}, true},
		{"explicit https", persistence.FileConnection{Protocol: "webdav", Port: intPtr(8080), ExtraConfig: map[string]any{"webdav_tls": true}}, false},
		{"not webdav", persistence.FileConnection{Protocol: "sftp", Port: intPtr(22)}, false},
	}
	for _, tc := range cases {
		if got := legacyPlainWebDAV(&tc.fc); got != tc.legacy {
			t.Errorf("%s: legacy plain = %t, want %t", tc.name, got, tc.legacy)
		}
	}

	legacy := persistence.FileConnection{ID: "fc-1", Name: "nas", Protocol: "webdav", Host: "nas.lan", Port: intPtr(8080), ExtraConfig: map[string]any{}}
	if _, err := (&Deps{}).buildConnectionConfig(&legacy); !errors.Is(err, errLegacyPlainWebDAV) {
		t.Fatalf("legacy plain WebDAV build error = %v, want upgrade notice", err)
	}
	rec := httptest.NewRecorder()
	writeFileProtocolError(rec, http.StatusInternalServerError, "failed to build connection config", errLegacyPlainWebDAV)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "LABTETHER_ALLOW_INSECURE_TRANSPORT") {
		t.Fatalf("legacy plain WebDAV error response = %d %s", rec.Code, rec.Body.String())
	}
	views := fileConnectionViews([]persistence.FileConnection{legacy})
	if len(views) != 1 || views[0].TransportNotice != legacyPlainWebDAVNotice {
		t.Fatalf("listed legacy connection lacks the upgrade notice: %+v", views)
	}
}
