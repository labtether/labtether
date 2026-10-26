package fileproto

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/studio-b12/gowebdav"
	"golang.org/x/net/webdav"
)

func TestWebDAVConnectRequiresExplicitInsecureOptIn(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	host, portText, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portText)
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	for _, tc := range []struct {
		name   string
		global string
		extra  map[string]any
	}{
		{"no opt in", "false", map[string]any{"webdav_tls": false}},
		{"local only", "false", map[string]any{"webdav_tls": false, "webdav_allow_cleartext": true}},
		{"global only", "true", map[string]any{"webdav_tls": false}},
		{"skip verify without global", "false", map[string]any{"webdav_tls_skip_verify": true}},
		{"cleartext with TLS", "true", map[string]any{"webdav_allow_cleartext": true}},
		{"invalid TLS flag", "true", map[string]any{"webdav_tls": "false"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", tc.global)
			client := &WebDAVClient{}
			err := client.Connect(context.Background(), ConnectionConfig{
				Host: host, Port: port, Username: "test-user", Secret: "test-password", ExtraConfig: tc.extra,
			})
			if err == nil || !strings.Contains(err.Error(), "webdav configuration") {
				t.Fatalf("expected a config rejection before sending credentials: %v", err)
			}
		})
	}
	if requests.Load() != 0 {
		t.Fatal("an unsafe config reached the server")
	}
}

func TestWebDAVEndpointIPv6AndPaths(t *testing.T) {
	endpoint, _, err := webDAVEndpoint(ConnectionConfig{
		Host: "2001:db8::1", Port: 8443, ExtraConfig: map[string]any{"webdav_base_path": "remote.php/dav/My Files/"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "https://[2001:db8::1]:8443/remote.php/dav/My Files" {
		t.Fatalf("invalid endpoint: %s", endpoint)
	}
	for _, host := range []string{"", "user@host", "host/path", "host?query", "host name"} {
		if _, _, err := webDAVEndpoint(ConnectionConfig{Host: host}); err == nil {
			t.Fatalf("invalid host accepted: %q", host)
		}
	}
}

func TestWebDAVPolicyRedirectsRemainOnConfiguredOrigin(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	for _, tc := range []struct {
		name     string
		location string
		allowed  bool
	}{
		{"path redirect", "/moved", true},
		{"implicit HTTPS port", "https://192.0.2.1/moved", true},
		{"other host", "https://192.0.2.2/moved", false},
		{"other port", "https://192.0.2.1:8443/moved", false},
		{"TLS downgrade", "http://192.0.2.1/moved", false},
		{"embedded credentials", "https://other:password@192.0.2.1/moved", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			transport, err := newWebDAVPolicyTransport("https://192.0.2.1:443", false, webDAVRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.Header.Get("Authorization") != "Bearer test-token" {
					t.Fatal("same-origin auth was lost")
				}
				payload, err := io.ReadAll(req.Body)
				if err != nil || string(payload) != "private-file" {
					t.Fatalf("unexpected payload: %q, %v", payload, err)
				}
				_ = req.Body.Close()
				response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody, Request: req}
				if calls == 1 {
					response.StatusCode = http.StatusTemporaryRedirect
					response.Header.Set("Location", tc.location)
				}
				return response, nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			request, _ := http.NewRequest(http.MethodPut, "https://192.0.2.1:443/file", bytes.NewBufferString("private-file"))
			request.Header.Set("Authorization", "Bearer test-token")
			response, err := (&http.Client{Transport: transport}).Do(request)
			if response != nil {
				_ = response.Body.Close()
			}
			if tc.allowed && (err != nil || calls != 2) {
				t.Fatalf("safe redirect failed: calls=%d err=%v", calls, err)
			}
			if !tc.allowed && (err == nil || calls != 1) {
				t.Fatalf("unsafe redirect reached transport: calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestWebDAVPolicyAppliesToAuthenticationRequests(t *testing.T) {
	calls := 0
	transport, err := newWebDAVPolicyTransport("https://192.0.2.1", false, webDAVRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusUnauthorized, Body: http.NoBody, Request: req,
			Header: http.Header{"Www-Authenticate": {"Passport1.4 challenge"}, "Location": {"https://192.0.2.2/"}},
		}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	client := gowebdav.NewClient("https://192.0.2.1", "test-user", "test-password")
	client.SetTransport(transport)
	if err := client.Connect(); err == nil {
		t.Fatal("expected cross-origin authentication rejection")
	}
	if calls != 1 {
		t.Fatalf("credential-bearing authentication request left configured origin: %d requests", calls)
	}
}

func TestWebDAVPolicyRechecksGlobalPolicyForReusedConnection(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	for _, tc := range []struct {
		endpoint   string
		skipVerify bool
	}{
		{"http://192.0.2.1", false},
		{"https://192.0.2.1", true},
	} {
		transport, err := newWebDAVPolicyTransport(tc.endpoint, tc.skipVerify, webDAVRoundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("request bypassed changed transport policy")
			return nil, nil
		}))
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
		request, _ := http.NewRequest(http.MethodGet, tc.endpoint, nil)
		if _, err := transport.RoundTrip(request); err == nil {
			t.Fatal("request ignored revoked insecure transport opt-in")
		}
		t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	}
}

func TestWebDAVTLSOnCustomPortSupportsAuthenticatedFileOperations(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	handler := &webdav.Handler{Prefix: "/dav/", FileSystem: webdav.NewMemFS(), LockSystem: webdav.NewMemLS()}
	var authenticated atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		user, password, ok := req.BasicAuth()
		if !ok || user != "test-user" || password != "test-password" {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		authenticated.Add(1)
		handler.ServeHTTP(w, req)
	}))
	defer server.Close()
	endpoint, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(endpoint.Port())
	client := &WebDAVClient{}
	if err := client.Connect(context.Background(), ConnectionConfig{
		Host: endpoint.Hostname(), Port: port, Username: "test-user", Secret: "test-password",
		ExtraConfig: map[string]any{"webdav_base_path": "/dav", "webdav_tls_skip_verify": true},
	}); err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Write(context.Background(), "/private file.txt", strings.NewReader("hello"), 5); err != nil {
		t.Fatal(err)
	}
	body, size, err := client.Read(context.Background(), "/private file.txt")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || size != 5 || string(payload) != "hello" {
		t.Fatalf("file round trip failed: size=%d body=%q err=%v", size, payload, err)
	}
	if err := client.Delete(context.Background(), "/private file.txt"); err != nil {
		t.Fatal(err)
	}
	if authenticated.Load() < 5 {
		t.Fatal("expected authenticated connect and file requests")
	}
}
