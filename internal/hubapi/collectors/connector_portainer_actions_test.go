package collectors_test

import (
	"bytes"
	"fmt"
	"github.com/labtether/labtether/internal/connectors/portainer"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleConnectorActionsPortainerRouteErrors(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	t.Run("portainer test method not allowed", func(t *testing.T) {
		sut := hubdeps.New(t)
		sut.ConnectorRegistry.Register(&mockPortainerConnector{})

		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer/test", nil)
		rec := httptest.NewRecorder()
		sut.CollectorsDeps().HandleConnectorActions(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
		testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "method not allowed")
	})

	t.Run("connector path not found", func(t *testing.T) {
		sut := hubdeps.New(t)
		req := httptest.NewRequest(http.MethodGet, "/connectors/", nil)
		rec := httptest.NewRecorder()
		sut.CollectorsDeps().HandleConnectorActions(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "connector path not found")
	})

	t.Run("invalid connector path", func(t *testing.T) {
		sut := hubdeps.New(t)
		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer", nil)
		rec := httptest.NewRecorder()
		sut.CollectorsDeps().HandleConnectorActions(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "invalid connector path")
	})

	t.Run("connector not registered", func(t *testing.T) {
		sut := hubdeps.New(t)
		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer/actions", nil)
		rec := httptest.NewRecorder()
		sut.CollectorsDeps().HandleConnectorActions(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "connector not registered")
	})

	t.Run("unknown connector action", func(t *testing.T) {
		sut := hubdeps.New(t)
		sut.ConnectorRegistry.Register(&mockPortainerConnector{})

		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer/not-real", nil)
		rec := httptest.NewRecorder()
		sut.CollectorsDeps().HandleConnectorActions(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "unknown connector action")
	})
}

func TestHandleConnectorActionsDispatchesPortainerTest(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/system/version":
			_, _ = w.Write([]byte(`{"ServerVersion":"2.21.0"}`))
		case "/api/endpoints":
			_, _ = w.Write([]byte(`[{"Id":1,"Name":"edge"}]`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "ptr-secret" {
			t.Fatalf("expected API key header")
		}
	}))
	defer mock.Close()

	sut := hubdeps.New(t)
	sut.ConnectorRegistry.Register(portainer.New())

	payload := []byte(fmt.Sprintf(`{
		"auth_method":"api_key",
		"base_url":"%s",
		"token_id":"svc@local!automation",
		"token_secret":"ptr-secret"
	}`, mock.URL))
	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	sut.CollectorsDeps().HandleConnectorActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleConnectorActionsPortainerTestRateLimit(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	sut := hubdeps.New(t)
	sut.ConnectorRegistry.Register(&mockPortainerConnector{})

	for i := 0; i < 12; i++ {
		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(`{"auth_method":"api_key"}`))
		req.RemoteAddr = "203.0.113.41:4401"
		rec := httptest.NewRecorder()
		sut.CollectorsDeps().HandleConnectorActions(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("request %d: expected 400 before rate limit, got %d", i+1, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(`{"auth_method":"api_key"}`))
	req.RemoteAddr = "203.0.113.41:4401"
	rec := httptest.NewRecorder()
	sut.CollectorsDeps().HandleConnectorActions(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after connector test burst, got %d body=%s", rec.Code, rec.Body.String())
	}
	testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "rate limit exceeded")
}
