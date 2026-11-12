package statusagg

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/labtether/labtether/internal/runtimesettings"
)

type fakeRuntimeOverrides map[string]string

func (f fakeRuntimeOverrides) ListRuntimeSettingOverrides() (map[string]string, error) {
	return f, nil
}

func (f fakeRuntimeOverrides) SaveRuntimeSettingOverrides(map[string]string) (map[string]string, error) {
	return f, nil
}

func (f fakeRuntimeOverrides) DeleteRuntimeSettingOverrides([]string) error { return nil }

// plainHTTPHub stands in for a hub that runs with LABTETHER_TLS_MODE=disabled
// behind a reverse proxy: /healthz over plain http. It counts requests so a
// test can show that a refused probe never connected.
func plainHTTPHub(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "")
	t.Setenv("LABTETHER_API_BASE_URL", "")
	t.Setenv("LABTETHER_AGENT_BASE_URL", "")
	return server, &requests
}

// With TLS ended by a reverse proxy the hub's own address is plain http. The
// Health page must not list the hub's own API as down because of that.
func TestHubChecksItsOwnPlainHTTPAddress(t *testing.T) {
	hub, requests := plainHTTPHub(t)
	t.Setenv("LABTETHER_API_BASE_URL", hub.URL)

	d := &Deps{Cache: &StatusCache{}}
	results := d.collectEndpointResults(context.Background())
	if len(results) != 1 {
		t.Fatalf("expected the hub's own endpoint only, got %#v", results)
	}
	got := results[0]
	if !got.OK || got.Status != "up" || got.Code != http.StatusOK || got.Error != "" {
		t.Fatalf("the hub's own plain http address must be up, got %#v", got)
	}
	if requests.Load() != 1 {
		t.Fatalf("expected one request to the hub, got %d", requests.Load())
	}
}

// The plain http path is only for the address the hub was deployed with. An
// address typed into Settings, or any other target, keeps the outbound policy
// and is refused before a connection is made.
func TestPlainHTTPStaysRefusedForOtherAddresses(t *testing.T) {
	hub, requests := plainHTTPHub(t)

	d := &Deps{
		Cache:        &StatusCache{},
		RuntimeStore: fakeRuntimeOverrides{runtimesettings.KeyRoutingAPIBaseURL: hub.URL},
	}
	fromSettings := d.collectEndpointResults(context.Background())
	if len(fromSettings) != 1 {
		t.Fatalf("expected one endpoint, got %#v", fromSettings)
	}
	other := probeEndpoint(context.Background(), endpointTarget{Name: "Other", URL: hub.URL + "/healthz"})

	for name, got := range map[string]EndpointResult{"typed into Settings": fromSettings[0], "other target": other} {
		if got.OK || got.Status != "down" || !strings.Contains(got.Error, "insecure url scheme") {
			t.Errorf("%s: plain http must stay refused, got %#v", name, got)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("a refused probe must not connect, got %d requests", requests.Load())
	}
}
