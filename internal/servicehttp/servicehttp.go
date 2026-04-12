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
	"strings"
	"time"
)

// DBPinger is satisfied by *pgxpool.Pool and any other type that can ping the database.
type DBPinger interface {
	Ping(ctx context.Context) error
}

// Config defines shared HTTP settings for LabTether services.
type Config struct {
	Name             string
	Port             string
	BindAddress      string // optional: listener bind address (default 0.0.0.0)
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

// Run starts a minimal HTTP server with common health endpoints.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Name == "" {
		cfg.Name = "service"
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if strings.TrimSpace(cfg.BindAddress) == "" {
		cfg.BindAddress = "0.0.0.0"
	}

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
		version := os.Getenv("APP_VERSION")
		if version == "" {
			version = "dev"
		}
		WriteJSON(w, http.StatusOK, map[string]any{
			"service":    cfg.Name,
			"version":    version,
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

	// Start HTTP→HTTPS redirect listener if configured.
	// Use WebSocket bypass so that the Next.js rewrite proxy (targeting the
	// HTTP port) can complete WebSocket upgrades for desktop/terminal streams
	// without being 301-redirected to HTTPS.
	if useTLS && cfg.RedirectHTTPPort != "" {
		redirectHandler := RedirectToHTTPSWithWebSocketBypass(cfg.HTTPSPort, mux)
		redirectServer := &http.Server{
			Addr:              net.JoinHostPort(cfg.BindAddress, cfg.RedirectHTTPPort),
			Handler:           redirectHandler,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       5 * time.Second,
			WriteTimeout:      5 * time.Second,
		}
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			if err := redirectServer.Shutdown(shutdownCtx); err != nil {
				log.Printf("%s redirect server shutdown error: %v", cfg.Name, err)
			}
		}()
		go func() {
			log.Printf("%s HTTP redirect listener on %s:%s → HTTPS :%d", cfg.Name, cfg.BindAddress, cfg.RedirectHTTPPort, cfg.HTTPSPort)
			if err := redirectServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("%s redirect server error: %v", cfg.Name, err)
			}
		}()
	}

	go func() {
		<-ctx.Done()
		timeoutSec := envIntOrDefault("LABTETHER_SHUTDOWN_TIMEOUT_SECONDS", 15)
		log.Printf("labtether: shutdown initiated, draining connections (timeout %ds)...", timeoutSec)
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(timeoutSec)*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("%s graceful shutdown error: %v", cfg.Name, err)
		}
		log.Printf("labtether: shutdown complete")
	}()

	var listenErr error
	if useTLS {
		log.Printf("%s listening on %s (TLS)", cfg.Name, addr)
		listenErr = server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
	} else {
		log.Printf("%s listening on %s", cfg.Name, addr)
		listenErr = server.ListenAndServe()
	}
	if listenErr != nil {
		if errors.Is(listenErr, http.ErrServerClosed) {
			return nil
		}
		return listenErr
	}

	return nil
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

// RedirectToHTTPSWithWebSocketBypass wraps RedirectToHTTPS while allowing
// WebSocket upgrade requests on stream endpoints to pass through unchanged.
// It also passes through /api/v1/tls/info so the dev frontend script can
// probe the backend's active TLS source over plain HTTP before it knows
// which cert to trust.
func RedirectToHTTPSWithWebSocketBypass(httpsPort int, wsHandler http.Handler) http.Handler {
	redirectHandler := RedirectToHTTPS(httpsPort)
	if wsHandler == nil {
		return redirectHandler
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Pass through TLS info on the HTTP port so the dev script can
		// probe the backend's active TLS source without needing TLS.
		if r.URL.Path == "/api/v1/tls/info" {
			wsHandler.ServeHTTP(w, r)
			return
		}
		if isWebSocketUpgradeRequest(r) && isRedirectWebSocketBypassPath(r.URL.Path) {
			wsHandler.ServeHTTP(w, r)
			return
		}
		redirectHandler.ServeHTTP(w, r)
	})
}

func isWebSocketUpgradeRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		return false
	}
	connectionHeader := strings.ToLower(r.Header.Get("Connection"))
	return strings.Contains(connectionHeader, "upgrade")
}

func isRedirectWebSocketBypassPath(path string) bool {
	if path == "/ws/events" {
		return true
	}
	return isSessionStreamPath(path, "/desktop/sessions/") || isSessionStreamPath(path, "/terminal/sessions/")
}

func isSessionStreamPath(path, prefix string) bool {
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	remainder := strings.TrimPrefix(path, prefix)
	parts := strings.Split(remainder, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] == "stream"
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

// envIntOrDefault reads an environment variable as an integer, returning the
// default value if the variable is unset or cannot be parsed.
func envIntOrDefault(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		log.Printf("labtether: invalid value for %s=%q, using default %d", key, v, defaultVal)
		return defaultVal
	}
	return n
}
