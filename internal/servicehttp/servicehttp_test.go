package servicehttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/certmgr"
)

func TestSecurityHeadersMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := SecurityHeaders(inner)
	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	expected := map[string]string{
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"Referrer-Policy":           "no-referrer",
		"Content-Security-Policy":   "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'",
	}

	for header, want := range expected {
		got := rec.Header().Get(header)
		if got != want {
			t.Errorf("header %s = %q, want %q", header, got, want)
		}
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestResolveBindAddressDefaultsToLoopback(t *testing.T) {
	if got := resolveBindAddress(""); got != "127.0.0.1" {
		t.Fatalf("default bind address = %q, want loopback", got)
	}
	if got := resolveBindAddress(" 0.0.0.0 "); got != "0.0.0.0" {
		t.Fatalf("explicit bind address = %q, want explicit value", got)
	}
}

func TestRedirectHandler_Redirects(t *testing.T) {
	handler := RedirectToHTTPS(8443)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/some/path?q=1", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMovedPermanently)
	}

	loc := rec.Header().Get("Location")
	want := "https://example.com:8443/some/path?q=1"
	if loc != want {
		t.Errorf("Location = %q, want %q", loc, want)
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Errorf("Referrer-Policy = %q, want %q", got, "no-referrer")
	}
}

func TestRedirectHandler_HealthzException(t *testing.T) {
	handler := RedirectToHTTPS(8443)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/healthz", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode JSON body: %v", err)
	}

	if body["status"] != "redirect_active" {
		t.Errorf("status = %q, want %q", body["status"], "redirect_active")
	}

	wantRedirect := "https on port 8443"
	if body["redirect"] != wantRedirect {
		t.Errorf("redirect = %q, want %q", body["redirect"], wantRedirect)
	}
}

func TestRedirectHandler_StripsPort(t *testing.T) {
	handler := RedirectToHTTPS(443)

	req := httptest.NewRequest(http.MethodGet, "http://example.com:8080/path", nil)
	req.Host = "example.com:8080"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMovedPermanently)
	}

	loc := rec.Header().Get("Location")
	want := "https://example.com:443/path"
	if loc != want {
		t.Errorf("Location = %q, want %q", loc, want)
	}
}

func TestRedirectHandlerWithTLSInfoPassthrough_DesktopStreamUpgradeRequiresTLS(t *testing.T) {
	wsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler := RedirectToHTTPSWithTLSInfoPassthrough(8443, wsHandler)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/desktop/sessions/sess-1/stream?ticket=abc", nil)
	req.Host = "example.com:8080"
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMovedPermanently)
	}
	want := "https://example.com:8443/desktop/sessions/sess-1/stream?ticket=abc"
	if got := rec.Header().Get("Location"); got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

func TestRedirectHandlerWithTLSInfoPassthrough_NonWebSocketStillRedirects(t *testing.T) {
	wsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler := RedirectToHTTPSWithTLSInfoPassthrough(8443, wsHandler)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/desktop/sessions/sess-1/stream?ticket=abc", nil)
	req.Host = "example.com:8080"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMovedPermanently)
	}
	want := "https://example.com:8443/desktop/sessions/sess-1/stream?ticket=abc"
	if got := rec.Header().Get("Location"); got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

func TestRedirectHandlerWithTLSInfoPassthrough_TLSInfoPassthrough(t *testing.T) {
	muxHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]any{
			"tls_source":  "built_in",
			"tls_enabled": true,
		})
	})
	handler := RedirectToHTTPSWithTLSInfoPassthrough(8443, muxHandler)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/v1/tls/info", nil)
	req.Host = "example.com:8080"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if resp["tls_source"] != "built_in" {
		t.Fatalf("tls_source = %v, want built_in", resp["tls_source"])
	}
}

func TestBearerAuthMiddleware(t *testing.T) {
	protected := BearerAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), "secret-token")

	req := httptest.NewRequest(http.MethodGet, "/agent/status", nil)
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	req = httptest.NewRequest(http.MethodGet, "/agent/status", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	req = httptest.NewRequest(http.MethodGet, "/agent/status", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("valid token status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("healthz bypass status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestEnvPositiveIntOrDefault(t *testing.T) {
	const key = "LABTETHER_TEST_POSITIVE_INT"
	tests := []struct {
		name string
		raw  string
		want int
	}{
		{name: "valid", raw: "42", want: 42},
		{name: "trimmed", raw: " 42 ", want: 42},
		{name: "malformedSuffix", raw: "42abc", want: 15},
		{name: "scientificNotation", raw: "1e2", want: 15},
		{name: "empty", raw: "", want: 15},
		{name: "zero", raw: "0", want: 15},
		{name: "negative", raw: "-1", want: 15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(key, tt.raw)
			if got := envPositiveIntOrDefault(key, 15); got != tt.want {
				t.Fatalf("envPositiveIntOrDefault(%q) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}

func TestEnvPositiveIntInRangeOrDefaultRejectsOutOfRangeValue(t *testing.T) {
	const key = "LABTETHER_TEST_POSITIVE_INT_RANGE"
	t.Setenv(key, "3601")

	if got := envPositiveIntInRangeOrDefault(key, 15, 3600); got != 15 {
		t.Fatalf("envPositiveIntInRangeOrDefault() = %d, want default 15", got)
	}
}

// helper: start a servicehttp server on an ephemeral port and return its base URL.
func startTestServer(t *testing.T, cfg Config) (baseURL string, cancel context.CancelFunc) {
	t.Helper()

	// Find a free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	cfg.BindAddress = "127.0.0.1"
	cfg.Port = fmt.Sprintf("%d", port)
	if cfg.Name == "" {
		cfg.Name = "test"
	}

	ctx, cancelFn := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- Run(ctx, cfg) }()

	// Wait for the server to accept connections.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, dialErr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 50*time.Millisecond)
		if dialErr == nil {
			conn.Close()
			return fmt.Sprintf("http://127.0.0.1:%d", port), cancelFn
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancelFn()
	t.Fatalf("server did not start within deadline")
	return "", nil
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("close free-port listener: %v", err)
	}
	return port
}

func waitForTCPServer(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	address := fmt.Sprintf("127.0.0.1:%d", port)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 50*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server %s did not start within deadline", address)
}

type shutdownServerStub struct {
	shutdownErr error
	closeErr    error
	closeCalls  int
}

func (s *shutdownServerStub) Shutdown(context.Context) error {
	return s.shutdownErr
}

func (s *shutdownServerStub) Close() error {
	s.closeCalls++
	return s.closeErr
}

func TestDrainHTTPServerForcesCloseAndPreservesTimeoutClassification(t *testing.T) {
	server := &shutdownServerStub{shutdownErr: context.DeadlineExceeded}

	err := drainHTTPServer("test", server, time.Second)
	if !errors.Is(err, ErrHTTPDrainIncomplete) {
		t.Fatalf("drain error = %v, want ErrHTTPDrainIncomplete", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("drain error = %v, want DeadlineExceeded", err)
	}
	if server.closeCalls != 1 {
		t.Fatalf("Close calls = %d, want 1", server.closeCalls)
	}
	if got := classifyHTTPDrainFailure(err); got != "timeout" {
		t.Fatalf("classification = %q, want timeout", got)
	}
}

func TestRunCancelsActiveHandlerContextBeforeGracefulDrain(t *testing.T) {
	t.Setenv("LABTETHER_SHUTDOWN_TIMEOUT_SECONDS", "2")
	port := freeTCPPort(t)
	started := make(chan struct{})
	handlerStopped := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, Config{
			Name:        "shutdown-context-test",
			BindAddress: "127.0.0.1",
			Port:        fmt.Sprintf("%d", port),
			ExtraHandlers: map[string]http.HandlerFunc{
				"/slow": func(w http.ResponseWriter, r *http.Request) {
					close(started)
					<-r.Context().Done()
					close(handlerStopped)
				},
			},
		})
	}()
	waitForTCPServer(t, port)

	requestDone := make(chan struct{})
	go func() {
		resp, _ := http.Get(fmt.Sprintf("http://127.0.0.1:%d/slow", port))
		if resp != nil {
			_ = resp.Body.Close()
		}
		close(requestDone)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("slow handler did not start")
	}
	cancel()

	select {
	case <-handlerStopped:
	case <-time.After(time.Second):
		t.Fatal("active handler context was not canceled")
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run returned error after context-aware handler drain: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after context-aware handler stopped")
	}
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("request did not finish")
	}
}

func TestRunPropagatesShutdownTimeoutAfterForcedClose(t *testing.T) {
	t.Setenv("LABTETHER_SHUTDOWN_TIMEOUT_SECONDS", "1")
	port := freeTCPPort(t)
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, Config{
			Name:        "shutdown-timeout-test",
			BindAddress: "127.0.0.1",
			Port:        fmt.Sprintf("%d", port),
			ExtraHandlers: map[string]http.HandlerFunc{
				"/blocked": func(http.ResponseWriter, *http.Request) {
					close(started)
					<-release
				},
			},
		})
	}()
	waitForTCPServer(t, port)

	requestDone := make(chan struct{})
	go func() {
		resp, _ := http.Get(fmt.Sprintf("http://127.0.0.1:%d/blocked", port))
		if resp != nil {
			_ = resp.Body.Close()
		}
		close(requestDone)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("blocked handler did not start")
	}
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, ErrHTTPDrainIncomplete) {
			t.Fatalf("Run error = %v, want ErrHTTPDrainIncomplete", err)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Run error = %v, want DeadlineExceeded", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after forced close")
	}

	releaseOnce.Do(func() { close(release) })
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("forced-close request did not finish")
	}
}

func TestRunWaitsForActiveMainHandlerShutdown(t *testing.T) {
	t.Setenv("LABTETHER_SHUTDOWN_TIMEOUT_SECONDS", "2")
	port := freeTCPPort(t)
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, Config{
			Name:        "shutdown-main-test",
			BindAddress: "127.0.0.1",
			Port:        fmt.Sprintf("%d", port),
			ExtraHandlers: map[string]http.HandlerFunc{
				"/slow": func(w http.ResponseWriter, _ *http.Request) {
					close(started)
					<-release
					w.WriteHeader(http.StatusNoContent)
				},
			},
		})
	}()
	waitForTCPServer(t, port)

	requestDone := make(chan error, 1)
	go func() {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/slow", port))
		if resp != nil {
			_ = resp.Body.Close()
		}
		requestDone <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("slow main handler did not start")
	}
	cancel()

	select {
	case err := <-errCh:
		t.Fatalf("Run returned before active main handler drained: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-requestDone; err != nil {
		t.Fatalf("slow main request failed: %v", err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run returned error after main drain: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after main handler drained")
	}
}

func TestRunWaitsForActiveRedirectHandlerShutdown(t *testing.T) {
	t.Setenv("LABTETHER_SHUTDOWN_TIMEOUT_SECONDS", "2")
	mainPort := freeTCPPort(t)
	redirectPort := freeTCPPort(t)
	certs, err := certmgr.Provision(t.TempDir(), "127.0.0.1")
	if err != nil {
		t.Fatalf("provision test certificate: %v", err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, Config{
			Name:             "shutdown-redirect-test",
			BindAddress:      "127.0.0.1",
			Port:             fmt.Sprintf("%d", mainPort),
			TLSCertFile:      certs.ServerCertPath,
			TLSKeyFile:       certs.ServerKeyPath,
			RedirectHTTPPort: fmt.Sprintf("%d", redirectPort),
			HTTPSPort:        mainPort,
			ExtraHandlers: map[string]http.HandlerFunc{
				"/api/v1/tls/info": func(w http.ResponseWriter, _ *http.Request) {
					close(started)
					<-release
					w.WriteHeader(http.StatusNoContent)
				},
			},
		})
	}()
	waitForTCPServer(t, mainPort)
	waitForTCPServer(t, redirectPort)

	requestDone := make(chan error, 1)
	go func() {
		resp, requestErr := http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/v1/tls/info", redirectPort))
		if resp != nil {
			_ = resp.Body.Close()
		}
		requestDone <- requestErr
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("slow redirect handler did not start")
	}
	cancel()

	select {
	case err := <-errCh:
		t.Fatalf("Run returned before active redirect handler drained: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-requestDone; err != nil {
		t.Fatalf("slow redirect request failed: %v", err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run returned error after redirect drain: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after redirect handler drained")
	}
}

func TestReadyz_NoCheckReturnsReady(t *testing.T) {
	base, cancel := startTestServer(t, Config{})
	defer cancel()

	resp, err := http.Get(base + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["ready"] != true {
		t.Fatalf("ready = %v, want true", body["ready"])
	}
}

func TestVersionUsesConfiguredBuildVersionWhenEnvironmentIsUnset(t *testing.T) {
	t.Setenv("APP_VERSION", "")
	base, cancel := startTestServer(t, Config{Version: "qa-exact-build"})
	defer cancel()

	resp, err := http.Get(base + "/version")
	if err != nil {
		t.Fatalf("GET /version: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := body["version"]; got != "qa-exact-build" {
		t.Fatalf("version = %v, want qa-exact-build", got)
	}
}

func TestReadyz_CheckPassesReturnsReady(t *testing.T) {
	base, cancel := startTestServer(t, Config{
		ReadinessCheck: func() error { return nil },
	})
	defer cancel()

	resp, err := http.Get(base + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["ready"] != true {
		t.Fatalf("ready = %v, want true", body["ready"])
	}
}

func TestReadyz_CheckFailsReturns503(t *testing.T) {
	base, cancel := startTestServer(t, Config{
		ReadinessCheck: func() error {
			return errors.New("database: connection refused")
		},
	})
	defer cancel()

	resp, err := http.Get(base + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["ready"] != false {
		t.Fatalf("ready = %v, want false", body["ready"])
	}
	if body["error"] != "readiness check failed" {
		t.Fatalf("error = %v, want %q", body["error"], "readiness check failed")
	}
}
