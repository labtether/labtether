package portainer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestExecWebSocketURLUsesCurrentAndLegacyExecIdentifiersWithoutLeakingJWT(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	const jwt = "sensitive-jwt"
	execID := strings.Repeat("a", portainerExecIDLength)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"jwt":"` + jwt + `"}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Username: "admin", Password: "secret"})
	wsURL, token, err := client.ExecWebSocketURL(context.Background(), 7, execID)
	if err != nil {
		t.Fatalf("ExecWebSocketURL: %v", err)
	}
	if token != jwt {
		t.Fatalf("token = %q, want JWT returned by auth", token)
	}
	parsed, err := url.Parse(wsURL)
	if err != nil {
		t.Fatalf("parse websocket URL: %v", err)
	}
	if parsed.Scheme != "ws" || parsed.Query().Get("endpointId") != "7" || parsed.Query().Get("id") != execID || parsed.Query().Get("execId") != execID {
		t.Fatalf("unexpected websocket URL %q", wsURL)
	}
	if strings.Contains(wsURL, jwt) || parsed.Query().Get("token") != "" {
		t.Fatalf("websocket URL leaked JWT: %q", wsURL)
	}
}

func TestDecodePortainerExecIDAcceptsBoundedCurrentAndLegacyShapes(t *testing.T) {
	execID := strings.Repeat("a", portainerExecIDLength)
	for _, payload := range []string{
		`{"Id":"` + execID + `"}`,
		`{"id":"` + execID + `"}`,
		`{"execId":"` + execID + `"}`,
		`{"Id":"` + execID + `","id":"` + execID + `","execId":"` + execID + `"}`,
	} {
		got, err := decodePortainerExecID([]byte(payload))
		if err != nil || got != execID {
			t.Fatalf("decodePortainerExecID(%s) = %q, %v", payload, got, err)
		}
	}
}

func TestDecodePortainerExecIDRejectsUnboundedMalformedAndConflictingShapes(t *testing.T) {
	valid := strings.Repeat("a", portainerExecIDLength)
	tests := []string{
		`{}`,
		`{"id":"short"}`,
		`{"id":"` + strings.Repeat("a", portainerExecIDLength+1) + `"}`,
		`{"id":"` + strings.Repeat("z", portainerExecIDLength) + `"}`,
		`{"id":123}`,
		`{"Id":"` + valid + `","execId":"` + strings.Repeat("b", portainerExecIDLength) + `"}`,
	}
	for _, payload := range tests {
		if got, err := decodePortainerExecID([]byte(payload)); err == nil {
			t.Fatalf("decodePortainerExecID(%s) unexpectedly returned %q", payload, got)
		}
	}
}

func allowInsecureTransportForPortainerTests(t *testing.T) {
	t.Helper()
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
}

func TestIsConfigured(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	tests := []struct {
		name   string
		client *Client
		want   bool
	}{
		{"nil client", nil, false},
		{"empty config", NewClient(Config{}), false},
		{"baseURL only", NewClient(Config{BaseURL: "https://localhost:9443"}), false},
		{"apiKey auth", NewClient(Config{BaseURL: "https://localhost:9443", APIKey: "key123"}), true},
		{"jwt auth", NewClient(Config{BaseURL: "https://localhost:9443", Username: "admin", Password: "pass"}), true},
		{"username without password", NewClient(Config{BaseURL: "https://localhost:9443", Username: "admin"}), false},
		{"password without username", NewClient(Config{BaseURL: "https://localhost:9443", Password: "pass"}), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.client.IsConfigured(); got != tt.want {
				t.Fatalf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewClientDefaults(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	client := NewClient(Config{
		BaseURL: "https://portainer.local:9443",
		APIKey:  "key",
	})
	if client.httpClient.Timeout != 10*time.Second {
		t.Fatalf("expected default timeout 10s, got %v", client.httpClient.Timeout)
	}
	transport, ok := client.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.httpClient.Transport)
	}
	if transport.MaxIdleConns != 10 {
		t.Fatalf("expected MaxIdleConns=10, got %d", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != 5 {
		t.Fatalf("expected MaxIdleConnsPerHost=5, got %d", transport.MaxIdleConnsPerHost)
	}
	if transport.IdleConnTimeout != 90*time.Second {
		t.Fatalf("expected IdleConnTimeout=90s, got %v", transport.IdleConnTimeout)
	}
	if transport.TLSClientConfig.MinVersion != 0x0303 { // TLS 1.2
		t.Fatalf("expected TLS 1.2 min, got %x", transport.TLSClientConfig.MinVersion)
	}
}

func TestCustomTimeout(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	client := NewClient(Config{
		BaseURL: "https://portainer.local:9443",
		APIKey:  "key",
		Timeout: 30 * time.Second,
	})
	if client.httpClient.Timeout != 30*time.Second {
		t.Fatalf("expected timeout 30s, got %v", client.httpClient.Timeout)
	}
}

func TestBaseURLTrailingSlash(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	client := NewClient(Config{
		BaseURL: "https://portainer.local:9443/",
		APIKey:  "key",
	})
	if strings.HasSuffix(client.baseURL, "/") {
		t.Fatalf("expected trailing slash removed, got %s", client.baseURL)
	}
}

func TestRequestAndDoRequestErrorPaths(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	t.Run("request body read failure", func(t *testing.T) {
		client := NewClient(Config{
			BaseURL: "https://portainer.local",
			APIKey:  "key",
		})
		_, err := client.request(context.Background(), http.MethodPost, "/api/system/version", failingReader{}, "application/json")
		if err == nil || !strings.Contains(err.Error(), "read request body") {
			t.Fatalf("expected read request body error, got %v", err)
		}
	})

	t.Run("request returns response status errors", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"downstream"}`))
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL: server.URL,
			APIKey:  "key",
		})
		_, err := client.request(context.Background(), http.MethodGet, "/api/system/version", nil, "")
		if err == nil || !strings.Contains(err.Error(), "portainer api returned 502") {
			t.Fatalf("expected status error, got %v", err)
		}
	})

	t.Run("request returns initial doRequest error", func(t *testing.T) {
		client := NewClient(Config{
			BaseURL: "https://portainer.local",
			APIKey:  "key",
		})
		client.baseURL = "://bad-url"

		_, err := client.request(context.Background(), http.MethodGet, "/api/system/version", nil, "")
		if err == nil || !strings.Contains(err.Error(), "invalid url") {
			t.Fatalf("expected initial doRequest error, got %v", err)
		}
	})

	t.Run("request retry propagates second doRequest error", func(t *testing.T) {
		var systemCalls atomic.Int32
		client := NewClient(Config{
			BaseURL:  "https://portainer.local",
			Username: "admin",
			Password: "secret",
		})
		client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/api/auth":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"jwt":"fresh-token"}`)),
					Header:     make(http.Header),
				}, nil
			case "/api/system/version":
				call := systemCalls.Add(1)
				if call == 1 {
					return &http.Response{
						StatusCode: http.StatusUnauthorized,
						Body:       io.NopCloser(strings.NewReader(`{"message":"expired token"}`)),
						Header:     make(http.Header),
					}, nil
				}
				return nil, errors.New("retry transport failure")
			default:
				return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
			}
		})

		_, err := client.request(context.Background(), http.MethodGet, "/api/system/version", nil, "")
		if err == nil || !strings.Contains(err.Error(), "retry transport failure") {
			t.Fatalf("expected retry doRequest error, got %v", err)
		}
	})

	t.Run("doRequest acquire jwt failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/auth" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"message":"bad auth"}`))
				return
			}
			t.Fatalf("unexpected path %s", r.URL.Path)
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL:  server.URL,
			Username: "admin",
			Password: "wrong",
		})

		_, _, err := client.doRequest(context.Background(), http.MethodGet, "api/system/version", nil, "")
		if err == nil || !strings.Contains(err.Error(), "acquire JWT") {
			t.Fatalf("expected acquire JWT error, got %v", err)
		}
	})

	t.Run("doRequest transport failure", func(t *testing.T) {
		client := NewClient(Config{
			BaseURL: "https://portainer.local",
			APIKey:  "key",
		})
		client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("dial failed")
		})

		_, _, err := client.doRequest(context.Background(), http.MethodGet, "/api/system/version", nil, "")
		if err == nil || !strings.Contains(err.Error(), "dial failed") {
			t.Fatalf("expected transport failure, got %v", err)
		}
	})

	t.Run("doRequest read body failure", func(t *testing.T) {
		client := NewClient(Config{
			BaseURL: "https://portainer.local",
			APIKey:  "key",
		})
		client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       errorReadCloser{},
				Header:     make(http.Header),
			}, nil
		})

		_, _, err := client.doRequest(context.Background(), http.MethodGet, "/api/system/version", nil, "")
		if err == nil || !strings.Contains(err.Error(), "forced read failure") {
			t.Fatalf("expected response read failure, got %v", err)
		}
	})

	t.Run("doRequest new request error", func(t *testing.T) {
		client := NewClient(Config{
			BaseURL: "https://portainer.local",
			APIKey:  "key",
		})
		client.baseURL = "://bad-url"

		_, _, err := client.doRequest(context.Background(), http.MethodGet, "/api/system/version", nil, "")
		if err == nil || !strings.Contains(err.Error(), "invalid url") {
			t.Fatalf("expected new request error, got %v", err)
		}
	})
}

func TestMarshalAndDecodeErrorPaths(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	client := NewClient(Config{
		BaseURL: "https://portainer.local",
		APIKey:  "key",
	})

	t.Run("post marshal error", func(t *testing.T) {
		_, err := client.post(context.Background(), "/api/test", map[string]any{"invalid": make(chan int)})
		if err == nil || !strings.Contains(err.Error(), "marshal request body") {
			t.Fatalf("expected marshal request body error, got %v", err)
		}
	})

	t.Run("put marshal error", func(t *testing.T) {
		_, err := client.put(context.Background(), "/api/test", map[string]any{"invalid": make(chan int)})
		if err == nil || !strings.Contains(err.Error(), "marshal request body") {
			t.Fatalf("expected marshal request body error, got %v", err)
		}
	})

	t.Run("post with nil body succeeds", func(t *testing.T) {
		var contentType string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			contentType = r.Header.Get("Content-Type")
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL: server.URL,
			APIKey:  "key",
		})
		if _, err := client.post(context.Background(), "/api/test", nil); err != nil {
			t.Fatalf("post with nil body failed: %v", err)
		}
		if contentType != "" {
			t.Fatalf("expected no content type for nil body, got %q", contentType)
		}
	})

	t.Run("post with json body sets content type", func(t *testing.T) {
		var contentType string
		var body map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			contentType = r.Header.Get("Content-Type")
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL: server.URL,
			APIKey:  "key",
		})
		if _, err := client.post(context.Background(), "/api/test", map[string]any{"hello": "world"}); err != nil {
			t.Fatalf("post with json body failed: %v", err)
		}
		if contentType != "application/json" {
			t.Fatalf("expected application/json content type, got %q", contentType)
		}
		if body["hello"] != "world" {
			t.Fatalf("unexpected request body: %#v", body)
		}
	})
}

func TestAPIResponseDecodeAndTruncationPaths(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	t.Run("GetVersion decode error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`not-json`))
		}))
		defer server.Close()

		client := NewClient(Config{BaseURL: server.URL, APIKey: "key"})
		_, err := client.GetVersion(context.Background())
		if err == nil || !strings.Contains(err.Error(), "decode version response") {
			t.Fatalf("expected version decode error, got %v", err)
		}
	})

	t.Run("GetEndpoints decode error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{invalid`))
		}))
		defer server.Close()

		client := NewClient(Config{BaseURL: server.URL, APIKey: "key"})
		_, err := client.GetEndpoints(context.Background())
		if err == nil || !strings.Contains(err.Error(), "decode endpoints response") {
			t.Fatalf("expected endpoints decode error, got %v", err)
		}
	})

	t.Run("GetEndpoints truncates to max", func(t *testing.T) {
		endpoints := make([]Endpoint, 0, maxEndpoints+5)
		for i := 1; i <= maxEndpoints+5; i++ {
			endpoints = append(endpoints, Endpoint{ID: i, Name: fmt.Sprintf("ep-%d", i), Type: 1, URL: "unix:///var/run/docker.sock", Status: 1})
		}
		payload, _ := json.Marshal(endpoints)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(payload)
		}))
		defer server.Close()

		client := NewClient(Config{BaseURL: server.URL, APIKey: "key"})
		got, err := client.GetEndpoints(context.Background())
		if err != nil {
			t.Fatalf("GetEndpoints failed: %v", err)
		}
		if len(got) != maxEndpoints {
			t.Fatalf("expected %d endpoints after truncation, got %d", maxEndpoints, len(got))
		}
	})

	t.Run("GetContainers decode error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{invalid`))
		}))
		defer server.Close()

		client := NewClient(Config{BaseURL: server.URL, APIKey: "key"})
		_, err := client.GetContainers(context.Background(), 1)
		if err == nil || !strings.Contains(err.Error(), "decode containers response") {
			t.Fatalf("expected containers decode error, got %v", err)
		}
	})

	t.Run("GetContainers truncates to max", func(t *testing.T) {
		containers := make([]Container, 0, maxContainersPerEndpoint+7)
		for i := 0; i < maxContainersPerEndpoint+7; i++ {
			containers = append(containers, Container{
				ID:    fmt.Sprintf("id-%d", i),
				Names: []string{fmt.Sprintf("/c-%d", i)},
			})
		}
		payload, _ := json.Marshal(containers)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(payload)
		}))
		defer server.Close()

		client := NewClient(Config{BaseURL: server.URL, APIKey: "key"})
		got, err := client.GetContainers(context.Background(), 1)
		if err != nil {
			t.Fatalf("GetContainers failed: %v", err)
		}
		if len(got) != maxContainersPerEndpoint {
			t.Fatalf("expected %d containers after truncation, got %d", maxContainersPerEndpoint, len(got))
		}
	})

	t.Run("GetStacks decode error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{invalid`))
		}))
		defer server.Close()

		client := NewClient(Config{BaseURL: server.URL, APIKey: "key"})
		_, err := client.GetStacks(context.Background())
		if err == nil || !strings.Contains(err.Error(), "decode stacks response") {
			t.Fatalf("expected stacks decode error, got %v", err)
		}
	})
}

type failingReader struct{}

func (failingReader) Read(p []byte) (int, error) {
	return 0, errors.New("forced read failure")
}

type errorReadCloser struct{}

func (errorReadCloser) Read(p []byte) (int, error) {
	return 0, errors.New("forced read failure")
}

func (errorReadCloser) Close() error { return nil }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
