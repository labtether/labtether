package servicehttp

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/securityruntime"
)

const maxShutdownTimeoutSeconds = 3600

const defaultBindAddress = "127.0.0.1"

// ErrHTTPDrainIncomplete means at least one HTTP server failed to prove that
// all active handlers stopped within its graceful-shutdown bound. Callers that
// own shared runtime state must treat this as a failed drain and terminate
// without releasing their ownership fence.
var ErrHTTPDrainIncomplete = errors.New("http connection drain incomplete")

// DBPinger is satisfied by *pgxpool.Pool and any other type that can ping the database.
type DBPinger interface {
	Ping(ctx context.Context) error
}

// Config defines shared HTTP settings for LabTether services.
type Config struct {
	Name             string
	Version          string // optional build version used when APP_VERSION is unset
	Port             string
	BindAddress      string // optional: listener bind address (default 127.0.0.1)
	AuthToken        string // #nosec G117 -- Runtime auth token config, not a hardcoded secret.
	ExtraHandlers    map[string]http.HandlerFunc
	TLSCertFile      string       // optional: path to TLS certificate file
	TLSKeyFile       string       // optional: path to TLS private key file
	DBPool           DBPinger     // optional: if set, /healthz pings the DB
	RedirectHTTPPort string       // if set, start an HTTP redirect listener on this port
	HTTPSPort        int          // the HTTPS port to redirect to
	ReadinessCheck   func() error // optional: returns nil if ready, error if not
	// GetCertificate is an optional TLS callback for dynamic certificate serving.
	// When set alongside TLSCertFile/TLSKeyFile, it is assigned to
	// tls.Config.GetCertificate so the server can hot-swap certs without restart.
	// Go's TLS stack calls GetCertificate preferentially over the static files.
	GetCertificate func(*tls.ClientHelloInfo) (*tls.Certificate, error)
}

// HTTPDrainError preserves the underlying graceful-shutdown and force-close
// errors while exposing a stable sentinel to runtime owners. Error intentionally
// reports only a bounded classification rather than transport details.
type HTTPDrainError struct {
	Server      string
	ShutdownErr error
	CloseErr    error
}

func (e *HTTPDrainError) Error() string {
	if e == nil {
		return ErrHTTPDrainIncomplete.Error()
	}
	return fmt.Sprintf(
		"%s: server=%s class=%s",
		ErrHTTPDrainIncomplete,
		strings.TrimSpace(e.Server),
		classifyHTTPDrainFailure(e.ShutdownErr),
	)
}

func (e *HTTPDrainError) Unwrap() []error {
	if e == nil {
		return nil
	}
	errs := []error{ErrHTTPDrainIncomplete}
	if e.ShutdownErr != nil {
		errs = append(errs, e.ShutdownErr)
	}
	if e.CloseErr != nil {
		errs = append(errs, e.CloseErr)
	}
	return errs
}

type httpServerShutdowner interface {
	Shutdown(context.Context) error
	Close() error
}

func drainHTTPServer(name string, server httpServerShutdowner, timeout time.Duration) error {
	if server == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	shutdownErr := server.Shutdown(shutdownCtx)
	if shutdownErr == nil {
		return nil
	}

	// Shutdown does not force active connections closed when its context
	// expires. Close them before returning so the owning process can terminate
	// promptly instead of continuing to serve after losing its runtime fence.
	closeErr := server.Close()
	forceClose := "complete"
	if closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) && !errors.Is(closeErr, net.ErrClosed) {
		forceClose = "failed"
	}
	log.Printf(
		"%s HTTP drain incomplete: class=%s force_close=%s",
		strings.TrimSpace(name),
		classifyHTTPDrainFailure(shutdownErr),
		forceClose,
	)
	return &HTTPDrainError{
		Server:      name,
		ShutdownErr: shutdownErr,
		CloseErr:    closeErr,
	}
}

func classifyHTTPDrainFailure(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, net.ErrClosed), errors.Is(err, http.ErrServerClosed):
		return "connection_closed"
	default:
		return "shutdown_error"
	}
}

// Run starts a minimal HTTP server with common health endpoints.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Name == "" {
		cfg.Name = "service"
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	cfg.BindAddress = resolveBindAddress(cfg.BindAddress)
	serverCtx, stopServers := context.WithCancel(ctx)
	defer stopServers()

	mux := http.NewServeMux()
	startedAt := time.Now().UTC()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		status := "ok"
		pgStatus := "ok"
		httpStatus := http.StatusOK
		if cfg.DBPool != nil {
			if err := cfg.DBPool.Ping(r.Context()); err != nil {
				status = "degraded"
				pgStatus = "unreachable"
				httpStatus = http.StatusServiceUnavailable
			}
		} else {
			pgStatus = "not_configured"
		}
		WriteJSON(w, httpStatus, map[string]any{
			"service":    cfg.Name,
			"status":     status,
			"postgres":   pgStatus,
			"goroutines": runtime.NumGoroutine(),
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
		})
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if cfg.ReadinessCheck != nil {
			if err := cfg.ReadinessCheck(); err != nil {
				log.Printf("readyz: check failed: %v", err)
				WriteJSON(w, http.StatusServiceUnavailable, map[string]any{
					"service": cfg.Name,
					"ready":   false,
					"error":   "readiness check failed",
				})
				return
			}
		}
		WriteJSON(w, http.StatusOK, map[string]any{
			"service": cfg.Name,
			"ready":   true,
		})
	})

	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		resolvedVersion := strings.TrimSpace(os.Getenv("APP_VERSION"))
		if resolvedVersion == "" {
			resolvedVersion = strings.TrimSpace(cfg.Version)
		}
		if resolvedVersion == "" {
			resolvedVersion = "dev"
		}
		WriteJSON(w, http.StatusOK, map[string]any{
			"service":    cfg.Name,
			"version":    resolvedVersion,
			"started_at": startedAt.Format(time.RFC3339),
		})
	})

	for path, handler := range cfg.ExtraHandlers {
		mux.HandleFunc(path, handler)
	}

	useTLS := cfg.TLSCertFile != "" && cfg.TLSKeyFile != ""

	// Build the middleware chain: mux → SecurityHeaders (if TLS) → BearerAuth (if token).
	var handler http.Handler = mux
	if useTLS {
		handler = SecurityHeaders(handler)
	}
	if strings.TrimSpace(cfg.AuthToken) != "" {
		handler = BearerAuth(handler, cfg.AuthToken)
	}

	addr := net.JoinHostPort(cfg.BindAddress, cfg.Port)
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		BaseContext:       func(net.Listener) context.Context { return serverCtx },
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	if useTLS {
		tlsCfg := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
		if cfg.GetCertificate != nil {
			tlsCfg.GetCertificate = cfg.GetCertificate
		}
		server.TLSConfig = tlsCfg
	}

	// Start HTTP→HTTPS redirect listener if configured. Remote-control
	// WebSockets must use the TLS listener; allowing an Upgrade through this
	// plaintext port exposes one-time tickets and terminal/desktop contents.
	redirectShutdownResult := make(chan error, 1)
	if useTLS && cfg.RedirectHTTPPort != "" {
		redirectHandler := RedirectToHTTPSWithTLSInfoPassthrough(cfg.HTTPSPort, mux)
		redirectServer := &http.Server{
			Addr:              net.JoinHostPort(cfg.BindAddress, cfg.RedirectHTTPPort),
			Handler:           redirectHandler,
			BaseContext:       func(net.Listener) context.Context { return serverCtx },
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       5 * time.Second,
			WriteTimeout:      5 * time.Second,
		}
		go func() {
			<-serverCtx.Done()
			redirectShutdownResult <- drainHTTPServer(cfg.Name+" redirect", redirectServer, 5*time.Second)
		}()
		go func() {
			log.Printf("%s HTTP redirect listener on %s:%s → HTTPS :%d", cfg.Name, cfg.BindAddress, cfg.RedirectHTTPPort, cfg.HTTPSPort)
			if err := redirectServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("%s redirect server error: %v", cfg.Name, err)
			}
		}()
	} else {
		redirectShutdownResult <- nil
	}

	serverShutdownResult := make(chan error, 1)
	go func() {
		<-serverCtx.Done()
		timeoutSec := envPositiveIntInRangeOrDefault("LABTETHER_SHUTDOWN_TIMEOUT_SECONDS", 15, maxShutdownTimeoutSeconds)
		log.Printf("labtether: shutdown initiated, draining connections (timeout %ds)...", timeoutSec)
		serverShutdownResult <- drainHTTPServer(cfg.Name, server, time.Duration(timeoutSec)*time.Second)
	}()

	var listenErr error
	if useTLS {
		log.Printf("%s listening on %s (TLS)", cfg.Name, addr)
		listenErr = server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
	} else {
		log.Printf("%s listening on %s", cfg.Name, addr)
		listenErr = server.ListenAndServe()
	}
	// ListenAndServe returns as soon as Shutdown closes the listener, before
	// active handlers necessarily finish. Keep ownership of the runtime until
	// both the main and redirect servers have completed their bounded drains.
	stopServers()
	serverShutdownErr := <-serverShutdownResult
	redirectShutdownErr := <-redirectShutdownResult
	drainErr := errors.Join(serverShutdownErr, redirectShutdownErr)
	// This only completes the HTTP listener/handler drain. Callers may still
	// need to stop background runtimes before releasing shared resources.
	if drainErr == nil {
		log.Printf("%s HTTP connection drain complete", cfg.Name)
	}
	if errors.Is(listenErr, http.ErrServerClosed) {
		listenErr = nil
	}
	return errors.Join(listenErr, drainErr)
}

func resolveBindAddress(raw string) string {
	if resolved := strings.TrimSpace(raw); resolved != "" {
		return resolved
	}
	return defaultBindAddress
}

// BearerAuth wraps a handler and enforces Authorization: Bearer token for all
// endpoints except health/readiness/version probes.
func BearerAuth(next http.Handler, token string) http.Handler {
	required := strings.TrimSpace(token)
	if required == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isUnauthenticatedProbePath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
		prefix := "Bearer "
		if !strings.HasPrefix(authHeader, prefix) {
			WriteError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		provided := strings.TrimSpace(strings.TrimPrefix(authHeader, prefix))
		if subtle.ConstantTimeCompare([]byte(provided), []byte(required)) != 1 {
			WriteError(w, http.StatusUnauthorized, "invalid bearer token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isUnauthenticatedProbePath(path string) bool {
	switch strings.TrimSpace(path) {
	case "/healthz", "/readyz", "/version":
		return true
	default:
		return false
	}
}

// SecurityHeaders wraps a handler to add standard security headers to all responses.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		next.ServeHTTP(w, r)
	})
}

// RedirectToHTTPS returns a handler that 301-redirects all requests to HTTPS.
// The /healthz endpoint is an exception: it returns 200 OK with JSON status
// so that Docker healthchecks (which can't follow redirects) still work.
// Security headers (X-Frame-Options, X-Content-Type-Options) are included on
// all responses, including redirects, to prevent clickjacking via HTTP.
func RedirectToHTTPS(httpsPort int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set security headers on all redirect responses to prevent clickjacking.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")

		if r.URL.Path == "/healthz" {
			WriteJSON(w, http.StatusOK, map[string]any{
				"status":   "redirect_active",
				"redirect": fmt.Sprintf("https on port %d", httpsPort),
			})
			return
		}

		host := r.Host
		// Strip existing port from Host header.
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}

		target := fmt.Sprintf("https://%s:%d%s", host, httpsPort, r.URL.RequestURI())
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})
}

// RedirectToHTTPSWithTLSInfoPassthrough redirects every application request,
// including WebSocket upgrades, while allowing the non-sensitive TLS metadata
// probe used during local development.
// probe the backend's active TLS source over plain HTTP before it knows
// which cert to trust.
func RedirectToHTTPSWithTLSInfoPassthrough(httpsPort int, tlsInfoHandler http.Handler) http.Handler {
	redirectHandler := RedirectToHTTPS(httpsPort)
	if tlsInfoHandler == nil {
		return redirectHandler
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Pass through TLS info on the HTTP port so the dev script can
		// probe the backend's active TLS source without needing TLS.
		if r.URL.Path == "/api/v1/tls/info" {
			tlsInfoHandler.ServeHTTP(w, r)
			return
		}
		redirectHandler.ServeHTTP(w, r)
	})
}

// RedirectToHTTPSWithWebSocketBypass is a compatibility alias. WebSocket
// bypass behavior was removed; all upgrades are redirected to TLS.
func RedirectToHTTPSWithWebSocketBypass(httpsPort int, tlsInfoHandler http.Handler) http.Handler {
	return RedirectToHTTPSWithTLSInfoPassthrough(httpsPort, tlsInfoHandler)
}

// WriteJSON writes a JSON response payload.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("WriteJSON: failed to encode response: %v", err)
	}
}

// WriteError writes a standard JSON error response.
// For 5xx responses, the original message is logged server-side and a generic
// message is returned to the client to avoid leaking internal details.
func WriteError(w http.ResponseWriter, status int, message string) {
	if status >= 500 {
		log.Printf("[error-sanitized] %d %s", status, message)
		message = "An internal error occurred."
	}
	WriteJSON(w, status, map[string]any{"error": message})
}

// envPositiveIntOrDefault reads an environment variable as a positive integer,
// returning the default value if the variable is unset, malformed, or non-positive.
func envPositiveIntOrDefault(key string, defaultVal int) int {
	return envPositiveIntInRangeOrDefault(key, defaultVal, 0)
}

func envPositiveIntInRangeOrDefault(key string, defaultVal int, maxVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n <= 0 {
		securityruntime.Logf("labtether: invalid value for %s=%q, using default %d", key, v, defaultVal)
		return defaultVal
	}
	if maxVal > 0 && n > maxVal {
		securityruntime.Logf("labtether: value for %s=%q exceeds maximum %d, using default %d", key, v, maxVal, defaultVal)
		return defaultVal
	}
	return n
}
