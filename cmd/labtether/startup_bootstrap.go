package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/hkdf"

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/certmgr"
	authpkg "github.com/labtether/labtether/internal/hubapi/auth"
	"github.com/labtether/labtether/internal/installstate"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

func runHub(ctx context.Context) error {
	databaseURL := envOrDefault("DATABASE_URL", persistence.DefaultDatabaseURL("localhost"))
	pgStore, err := persistence.NewPostgresStore(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("labtether failed to initialize postgres store: %w", err)
	}
	defer pgStore.Close()

	dataDir := envOrDefault("LABTETHER_DATA_DIR", "data")
	runtimeSecrets, err := resolveRuntimeInstallSecrets(newInstallStateStore(dataDir))
	if err != nil {
		return fmt.Errorf("labtether startup failed: install state init failed: %w", err)
	}

	authValidator := auth.NewTokenValidator(runtimeSecrets.OwnerToken, runtimeSecrets.APIToken)
	if !authValidator.Configured() {
		return fmt.Errorf("labtether startup failed: runtime auth token is not configured")
	}

	secretsManager, err := loadSecretsManager(runtimeSecrets.EncryptionKey)
	if err != nil {
		return fmt.Errorf("labtether startup failed: invalid runtime encryption key: %w", err)
	}
	if secretsManager == nil {
		log.Printf("labtether warning: runtime encryption key not set; credential encryption endpoints are disabled")
	}

	registry := buildConnectorRegistry()

	// Policy state (inline from policy service).
	policyCfg := loadPolicyConfigFromEnv()
	policyState := newPolicyRuntimeState(policyCfg)
	go refreshPolicyRuntimeSettingsDirect(ctx, pgStore, policyState)
	go refreshSecurityRuntimeSettingsDirect(ctx, pgStore)

	// Bootstrap admin user for MVP single-user auth.
	if err := bootstrapAdminUser(pgStore); err != nil {
		return fmt.Errorf("labtether startup failed: admin bootstrap failed: %w", err)
	}

	oidcProvider, oidcAutoProvision, err := loadOIDCProviderFromEnv(ctx)
	if err != nil {
		log.Printf("labtether auth: warning: oidc initialization failed: %v (starting with oidc disabled)", err)
	}
	oidcRef := authpkg.NewOIDCProviderRef(oidcProvider, oidcAutoProvision)

	srv := newAPIServer(pgStore, secretsManager, policyState, registry, authValidator, oidcRef, newInstallStateStore(dataDir))
	srv.dataDir = dataDir

	// Derive TOTP encryption key for 2FA secret storage.
	totpKey, err := deriveTOTPKey(runtimeSecrets.EncryptionKey)
	if err != nil {
		return fmt.Errorf("labtether startup failed: could not derive TOTP encryption key: %w", err)
	}
	srv.totpEncryptionKey = totpKey

	configureServerRuntime(srv, registry, secretsManager, pgStore)
	initMetricsExport(srv, pgStore)

	// Start periodic challenge token cleanup to prevent unbounded memory growth.
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				srv.challengeStore.Cleanup()
			}
		}
	}()

	worker := initializeWorkerSubsystem(ctx, srv, pgStore)
	startRuntimeLoops(ctx, srv, pgStore, worker.state, worker.retentionTracker)

	// --- TLS mode resolution ---
	tlsMode := envOrDefault("LABTETHER_TLS_MODE", "auto")
	tlsCertFile := envOrDefault("LABTETHER_TLS_CERT", "")
	tlsKeyFile := envOrDefault("LABTETHER_TLS_KEY", "")
	httpsPort := envOrDefault("LABTETHER_HTTPS_PORT", "8443")
	httpPort := envOrDefault("LABTETHER_HTTP_PORT", envOrDefault("API_PORT", "8080"))

	// External cert env vars override mode to "external"
	if tlsCertFile != "" && tlsKeyFile != "" && tlsMode == "auto" {
		tlsMode = "external"
	}

	defaultTLSMode := tlsMode
	defaultTLSSource := tlsSourceDisabled
	switch defaultTLSMode {
	case "auto":
		defaultTLSSource = tlsSourceBuiltIn
	case "external":
		defaultTLSSource = tlsSourceDeploymentExternal
	}

	var port string
	switch defaultTLSMode {
	case "disabled":
		port = httpPort
		log.Printf("labtether: TLS disabled — serving HTTP on :%s", port)

	case "external":
		if tlsCertFile == "" || tlsKeyFile == "" {
			return fmt.Errorf("labtether: TLS mode 'external' requires LABTETHER_TLS_CERT and LABTETHER_TLS_KEY")
		}
		port = httpsPort
		srv.tlsState.Enabled = true
		log.Printf("labtether: TLS enabled (external cert=%s, key=%s) — serving HTTPS on :%s", tlsCertFile, tlsKeyFile, port)

	default: // "auto"
		certsDir := dataDir + "/certs"

		// Always provision the built-in self-signed CA so LAN-only agents
		// can still pin it, regardless of which cert the HTTPS listener uses.
		builtInResult, builtInErr := certmgr.Provision(certsDir)
		if builtInErr != nil {
			return fmt.Errorf("labtether: built-in CA provisioning failed: %w", builtInErr)
		}
		srv.tlsState.CACertPEM = builtInResult.CACertPEM
		srv.tlsState.CertReloader = builtInResult.Reloader
		go builtInResult.Reloader.Run(ctx)
		shareCACert(builtInResult.CACertPEM, "/ca")

		// Prefer Tailscale cert (publicly trusted) over self-signed.
		if tsCertPath, tsKeyPath, tsDomain, tsErr := provisionTailscaleCert(certsDir); tsErr == nil {
			tlsCertFile = tsCertPath
			tlsKeyFile = tsKeyPath
			defaultTLSSource = tlsSourceTailscale
			tsReloader := newTailscaleCertReloader(tsCertPath, tsKeyPath, tsDomain, certsDir)
			srv.tlsState.TailscaleCertReloader = tsReloader
			go tsReloader.Run(ctx)
			log.Printf("labtether: TLS using Tailscale cert for %s — serving HTTPS on :%s", tsDomain, httpsPort)
		} else {
			log.Printf("labtether: Tailscale cert not available (%v), using built-in self-signed", tsErr)
			tlsCertFile = builtInResult.ServerCertPath
			tlsKeyFile = builtInResult.ServerKeyPath
		}

		port = httpsPort
		srv.tlsState.Enabled = true
	}

	srv.tlsState.Mode = defaultTLSMode
	srv.tlsState.Source = defaultTLSSource
	srv.tlsState.CertFile = tlsCertFile
	srv.tlsState.KeyFile = tlsKeyFile
	if v, err := strconv.Atoi(httpsPort); err != nil {
		return fmt.Errorf("labtether: invalid LABTETHER_HTTPS_PORT %q: %w", httpsPort, err)
	} else {
		srv.tlsState.HttpsPort = v
	}
	if v, err := strconv.Atoi(httpPort); err != nil {
		return fmt.Errorf("labtether: invalid LABTETHER_HTTP_PORT %q: %w", httpPort, err)
	} else {
		srv.tlsState.HttpPort = v
	}
	srv.tlsState.DefaultMode = defaultTLSMode
	srv.tlsState.DefaultSource = defaultTLSSource
	srv.tlsState.DefaultCertFile = tlsCertFile
	srv.tlsState.DefaultKeyFile = tlsKeyFile
	srv.tlsState.DefaultCAPEM = append([]byte(nil), srv.tlsState.CACertPEM...)

	if srv.tlsState.Enabled {
		srv.tlsState.CertSwitcher = &hubCertificateSwitcher{}
		if tsrl, ok := srv.tlsState.TailscaleCertReloader.(*tailscaleCertReloader); ok && tsrl != nil {
			srv.tlsState.CertSwitcher.SetProvider(tsrl.GetCertificate)
			srv.tlsState.DefaultGetCertificate = tsrl.GetCertificate
		} else if srv.tlsState.CertReloader != nil {
			srv.tlsState.CertSwitcher.SetProvider(srv.tlsState.CertReloader.GetCertificate)
			srv.tlsState.DefaultGetCertificate = srv.tlsState.CertReloader.GetCertificate
		} else {
			staticProvider, err := newStaticHubCertificateProvider(tlsCertFile, tlsKeyFile)
			if err != nil {
				return fmt.Errorf("labtether: load active TLS key pair: %w", err)
			}
			srv.tlsState.CertSwitcher.SetProvider(staticProvider.GetCertificate)
			srv.tlsState.DefaultGetCertificate = staticProvider.GetCertificate
		}
	}

	if override, ok, err := loadPersistedTLSOverride(pgStore, secretsManager); err != nil {
		log.Printf("labtether warning: ignoring persisted TLS override: %v", err)
	} else if ok {
		overrideCertPath, overrideKeyPath, err := materializeUploadedTLSFiles(dataDir, override.CertPEM, override.KeyPEM)
		if err != nil {
			log.Printf("labtether warning: failed to materialize persisted TLS override: %v", err)
		} else {
			overrideProvider, providerErr := newStaticHubCertificateProvider(overrideCertPath, overrideKeyPath)
			if providerErr != nil {
				log.Printf("labtether warning: failed to load persisted TLS override: %v", providerErr)
			} else {
				srv.tlsState.Enabled = true
				srv.tlsState.Mode = "external"
				srv.tlsState.Source = tlsSourceUIUploaded
				srv.tlsState.CertFile = overrideCertPath
				srv.tlsState.KeyFile = overrideKeyPath
				srv.tlsState.CACertPEM = nil
				tlsCertFile = overrideCertPath
				tlsKeyFile = overrideKeyPath
				port = httpsPort
				if srv.tlsState.CertSwitcher == nil {
					srv.tlsState.CertSwitcher = &hubCertificateSwitcher{}
				}
				srv.tlsState.CertSwitcher.SetProvider(overrideProvider.GetCertificate)
				log.Printf("labtether: TLS UI override loaded — serving HTTPS on :%s", port)
			}
		}
	}

	portInt, portErr := strconv.Atoi(port)
	if portErr != nil || portInt < 1 || portInt > 65535 {
		return fmt.Errorf("labtether: invalid port %q (must be 1-65535)", port)
	}
	if strings.TrimSpace(srv.externalURL) != "" {
		if _, ok := srv.sanitizedExternalHubURL(); !ok {
			log.Printf("labtether warning: ignoring invalid or insecure LABTETHER_EXTERNAL_URL=%q", srv.externalURL)
		}
	}

	// Advertise the hub via mDNS/Bonjour so that iOS companion apps can
	// discover it automatically on the local network.
	//
	// On macOS, mDNS/Bonjour triggers a Local Network privacy prompt.
	// When running in a headless context (tmux, launchd), macOS cannot
	// display the prompt and auto-denies it, which cascades to block ALL
	// local network access for the entire process. Disable mDNS when
	// LABTETHER_DISABLE_MDNS=true to avoid this.
	if !envOrDefaultBool("LABTETHER_DISABLE_MDNS", false) {
		startMDNSAdvertiser(ctx, portInt)
	} else {
		log.Printf("labtether: mDNS advertiser disabled (LABTETHER_DISABLE_MDNS=true)")
	}

	redirectPort := ""
	httpsPortInt := 0
	if srv.tlsState.Enabled {
		redirectPort = httpPort
		if v, err := strconv.Atoi(port); err != nil {
			return fmt.Errorf("labtether: invalid HTTPS port %q for redirect: %w", port, err)
		} else {
			httpsPortInt = v
		}
	}

	handlers := srv.buildHTTPHandlers(worker.state, worker.retentionTracker, worker.counters)

	// Wrap every handler with gzip compression. The middleware is path-aware
	// and skips WebSocket upgrade paths and known binary-framed stream routes
	// so that compression is only applied to regular JSON API responses.
	for path, h := range handlers {
		handlers[path] = gzipMiddleware(h).ServeHTTP
	}

	// Wrap every handler with CORS middleware. This runs as the outermost
	// layer so that OPTIONS preflight requests receive proper CORS headers
	// and a 204 response without passing through auth middleware.
	for path, h := range handlers {
		wrapped := srv.corsMiddleware(http.HandlerFunc(h))
		handlers[path] = wrapped.ServeHTTP
	}

	httpCfg := servicehttp.Config{
		Name:             "labtether",
		Port:             port,
		TLSCertFile:      tlsCertFile,
		TLSKeyFile:       tlsKeyFile,
		RedirectHTTPPort: redirectPort,
		HTTPSPort:        httpsPortInt,
		ExtraHandlers:    handlers,
		DBPool:           pgStore.Pool(),
	}
	if srv.tlsState.CertSwitcher != nil {
		httpCfg.GetCertificate = srv.tlsState.CertSwitcher.GetCertificate
	}

	if err := servicehttp.Run(ctx, httpCfg); err != nil {
		return fmt.Errorf("service run failed: %w", err)
	}
	return nil
}

type runtimeInstallSecrets struct {
	OwnerToken    string
	APIToken      string // #nosec G117 -- Runtime install secret, not a hardcoded credential.
	EncryptionKey string
}

func resolveRuntimeInstallSecrets(store *installstate.Store) (runtimeInstallSecrets, error) {
	var resolved runtimeInstallSecrets
	if store == nil {
		return resolved, errors.New("install state store is required")
	}

	meta, persisted, exists, err := store.Load()
	if err != nil {
		return resolved, err
	}

	envOwnerToken := strings.TrimSpace(os.Getenv("LABTETHER_OWNER_TOKEN"))
	envAPIToken := strings.TrimSpace(os.Getenv("LABTETHER_API_TOKEN"))
	envEncryptionKey := strings.TrimSpace(os.Getenv("LABTETHER_ENCRYPTION_KEY"))
	envPostgresPassword := strings.TrimSpace(os.Getenv("POSTGRES_PASSWORD"))

	resolved.OwnerToken = strings.TrimSpace(persisted.OwnerToken)
	if isWellKnownPlaceholder(resolved.OwnerToken) {
		log.Printf("labtether: WARNING: persisted owner token matches a well-known dev placeholder — regenerating")
		resolved.OwnerToken = ""
	}
	if envOwnerToken != "" {
		resolved.OwnerToken = envOwnerToken
	}
	if resolved.OwnerToken == "" {
		token, err := generateHexToken(32)
		if err != nil {
			return runtimeInstallSecrets{}, fmt.Errorf("generate owner token: %w", err)
		}
		resolved.OwnerToken = token
	}

	resolved.APIToken = strings.TrimSpace(persisted.APIToken)
	if isWellKnownPlaceholder(resolved.APIToken) {
		log.Printf("labtether: WARNING: persisted API token matches a well-known dev placeholder — regenerating")
		resolved.APIToken = ""
	}
	if envAPIToken != "" {
		resolved.APIToken = envAPIToken
	}
	if resolved.APIToken == "" {
		token, err := generateHexToken(32)
		if err != nil {
			return runtimeInstallSecrets{}, fmt.Errorf("generate api token: %w", err)
		}
		resolved.APIToken = token
	}

	resolved.EncryptionKey = strings.TrimSpace(persisted.EncryptionKey)
	if isWellKnownPlaceholderKey(resolved.EncryptionKey) {
		log.Printf("labtether: WARNING: persisted encryption key matches a well-known dev placeholder — regenerating")
		resolved.EncryptionKey = ""
	}
	if envEncryptionKey != "" {
		resolved.EncryptionKey = envEncryptionKey
	}
	if resolved.EncryptionKey == "" {
		key, err := generateBase64Key(32)
		if err != nil {
			return runtimeInstallSecrets{}, fmt.Errorf("generate encryption key: %w", err)
		}
		resolved.EncryptionKey = key
	}

	if _, err := loadSecretsManager(resolved.EncryptionKey); err != nil {
		return runtimeInstallSecrets{}, fmt.Errorf("validate encryption key: %w", err)
	}

	now := time.Now().UTC()
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
	}
	meta.UpdatedAt = now

	nextSecrets := installstate.Secrets{
		OwnerToken:       resolved.OwnerToken,
		APIToken:         resolved.APIToken,
		EncryptionKey:    resolved.EncryptionKey,
		PostgresPassword: strings.TrimSpace(persisted.PostgresPassword),
	}
	if envPostgresPassword != "" {
		nextSecrets.PostgresPassword = envPostgresPassword
	}
	if !exists || persisted != nextSecrets || meta.SchemaVersion != 1 {
		if err := store.Save(meta, nextSecrets); err != nil {
			return runtimeInstallSecrets{}, err
		}
		if exists {
			log.Printf("labtether: install state secrets refreshed in %s", store.Root())
		} else {
			log.Printf("labtether: install state initialized in %s", store.Root())
		}
	}
	if err := writeRuntimeAPITokenFile(resolved.APIToken); err != nil {
		return runtimeInstallSecrets{}, err
	}

	return resolved, nil
}

func writeRuntimeAPITokenFile(token string) error {
	path := strings.TrimSpace(os.Getenv("LABTETHER_API_TOKEN_FILE"))
	if path == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil { // #nosec G703 -- Directory is derived from the fixed runtime token file path.
		return fmt.Errorf("create runtime api token directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(token)), 0o600); err != nil { // #nosec G703 -- Runtime token file path is package-controlled state.
		return fmt.Errorf("write runtime api token file: %w", err)
	}
	return nil
}

func generateHexToken(numBytes int) (string, error) {
	raw := make([]byte, numBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func generateBase64Key(numBytes int) (string, error) {
	raw := make([]byte, numBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// deriveTOTPKey returns a stable 32-byte AES-256 key for encrypting TOTP secrets.
//
// Key derivation priority:
//  1. LABTETHER_TOTP_KEY env var — base64-decoded directly (must be 32 bytes).
//  2. LABTETHER_ENCRYPTION_KEY env var — derive via HKDF-SHA256 with info "labtether-totp-v1".
//  3. Random ephemeral key — logs a warning because 2FA secrets will not survive restart.
func deriveTOTPKey(runtimeEncryptionKey string) ([]byte, error) {
	// Option 1: explicit TOTP key override.
	if raw := strings.TrimSpace(os.Getenv("LABTETHER_TOTP_KEY")); raw != "" {
		key, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("LABTETHER_TOTP_KEY is not valid base64: %w", err)
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("LABTETHER_TOTP_KEY must decode to exactly 32 bytes (got %d)", len(key))
		}
		return key, nil
	}

	// Option 2: derive from the main encryption key via HKDF.
	if raw := strings.TrimSpace(runtimeEncryptionKey); raw != "" {
		master, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("runtime encryption key is not valid base64: %w", err)
		}
		r := hkdf.New(sha256.New, master, nil, []byte("labtether-totp-v1"))
		key := make([]byte, 32)
		if _, err := io.ReadFull(r, key); err != nil {
			return nil, fmt.Errorf("hkdf derive TOTP key: %w", err)
		}
		return key, nil
	}

	// Option 3: ephemeral random key — 2FA will break on restart.
	log.Printf("labtether WARNING: TOTP encryption key is ephemeral; 2FA secrets will not survive restart. Set LABTETHER_ENCRYPTION_KEY or LABTETHER_TOTP_KEY for persistence.")
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("rand.Read TOTP key: %w", err)
	}
	return key, nil
}

// wellKnownPlaceholderTokens are dev/example tokens that must never be used in
// real deployments. If persisted state contains one of these, the bootstrap
// regenerates a cryptographically random replacement.
var wellKnownPlaceholderTokens = map[string]bool{
	"labtether-owner-local-token": true,
}

// wellKnownPlaceholderKeys are dev/example encryption keys (base64) that must
// never be used in real deployments.
var wellKnownPlaceholderKeys = map[string]bool{
	"MDEyMzQ1Njc4OUFCQ0RFRjAxMjM0NTY3ODlBQkNERUY=": true,
}

func isWellKnownPlaceholder(token string) bool {
	return token != "" && wellKnownPlaceholderTokens[token]
}

func isWellKnownPlaceholderKey(key string) bool {
	return key != "" && wellKnownPlaceholderKeys[key]
}

// shareCACert copies the public CA cert into a share directory for sidecar
// services (agent/console) when available. In non-container local dev runs,
// the default path is often absent; skip quietly in that case.
func shareCACert(caCertPEM []byte, defaultShareDir string) {
	shareDir, explicit := os.LookupEnv("LABTETHER_CA_SHARE_DIR")
	if explicit {
		shareDir = strings.TrimSpace(shareDir)
		if shareDir == "" {
			log.Printf("labtether: warning: LABTETHER_CA_SHARE_DIR is empty; skipping CA share copy")
			return
		}
	} else {
		shareDir = strings.TrimSpace(defaultShareDir)
		if shareDir == "" {
			return
		}
		info, err := os.Stat(shareDir)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return
			}
			log.Printf("labtether: warning: could not stat CA share dir %s: %v", shareDir, err)
			return
		}
		if !info.IsDir() {
			log.Printf("labtether: warning: CA share path %s is not a directory; skipping", shareDir)
			return
		}
	}

	if err := os.MkdirAll(shareDir, 0750); err != nil {
		log.Printf("labtether: warning: could not create CA share dir %s: %v", shareDir, err)
		return
	}

	caPath := filepath.Join(shareDir, "ca.crt")
	if err := os.WriteFile(caPath, caCertPEM, 0644); err != nil { // #nosec G306 -- CA certificate is public trust material and intended to be readable by sidecars.
		log.Printf("labtether: warning: could not write CA to %s: %v", caPath, err)
	}
}
