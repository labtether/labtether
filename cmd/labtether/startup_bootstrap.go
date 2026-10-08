package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/certmgr"
	"github.com/labtether/labtether/internal/demo"
	authpkg "github.com/labtether/labtether/internal/hubapi/auth"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/secrets"
	"github.com/labtether/labtether/internal/servicehttp"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type oidcClientSecretMigrator interface {
	MigrateLegacyOIDCClientSecret(context.Context, *secrets.Manager) error
}

func migrateOIDCClientSecretAtStartup(ctx context.Context, migrator oidcClientSecretMigrator, manager *secrets.Manager) error {
	if err := migrator.MigrateLegacyOIDCClientSecret(ctx, manager); err != nil {
		return newStartupFailure(startupFailureEncryptionConfig, err)
	}
	return nil
}

func runHub(ctx context.Context) error {
	bindAddress, err := resolveHubBindAddress()
	if err != nil {
		return newStartupFailure(startupFailureServerRuntime, err)
	}
	if err := validateExplicitHubExposureAuth(
		bindAddress,
		os.Getenv("LABTETHER_OWNER_TOKEN"),
		os.Getenv("LABTETHER_API_TOKEN"),
	); err != nil {
		return newStartupFailure(startupFailureAuthConfiguration, err)
	}

	databaseURL := envOrDefault("DATABASE_URL", persistence.DefaultDatabaseURL("localhost"))
	pgStore, err := persistence.NewPostgresStore(ctx, databaseURL)
	if err != nil {
		return newDatabaseStartupFailure(err)
	}
	runtimeDrained := true
	httpConnectionsDrained := true
	defer func() {
		closeHubPostgresStore(runtimeDrained, pgStore)
	}()

	runtimeLease, err := pgStore.AcquireHubRuntimeLease(ctx)
	if err != nil {
		return newStartupFailure(startupFailureRuntimeOwnership, err)
	}
	defer func() {
		releaseHubRuntimeLease(runtimeDrained, runtimeLease)
	}()
	leaseCtx, cancelForLease := context.WithCancelCause(ctx)
	leaseMonitorDone := make(chan struct{})
	go func() {
		defer close(leaseMonitorDone)
		monitorHubRuntimeLease(
			leaseCtx,
			runtimeLease,
			hubRuntimeLeaseCheckInterval,
			hubRuntimeLeasePingTimeout,
			cancelForLease,
		)
	}()
	defer func() {
		cancelForLease(nil)
		<-leaseMonitorDone
	}()
	ctx = leaseCtx

	dataDir := envOrDefault("LABTETHER_DATA_DIR", "data")
	runtimeSecrets, err := resolveRuntimeInstallSecrets(newInstallStateStore(dataDir))
	if err != nil {
		return newStartupFailure(startupFailureInstallState, err)
	}
	if err := validateHubExposureAuth(bindAddress, runtimeSecrets); err != nil {
		return newStartupFailure(startupFailureAuthConfiguration, err)
	}

	authValidator := auth.NewTokenValidator(runtimeSecrets.OwnerToken, runtimeSecrets.APIToken)
	if !authValidator.Configured() {
		return newStartupFailure(startupFailureAuthConfiguration, errors.New("runtime auth token is not configured"))
	}

	secretsManager, err := loadSecretsManager(runtimeSecrets.EncryptionKey)
	if err != nil {
		return newStartupFailure(startupFailureEncryptionConfig, err)
	}
	if secretsManager == nil {
		log.Printf("labtether warning: runtime encryption key not set; credential encryption endpoints are disabled")
	}
	if err := migrateOIDCClientSecretAtStartup(ctx, pgStore, secretsManager); err != nil {
		return err
	}

	demoMode := envOrDefaultBool("LABTETHER_DEMO_MODE", false)
	if demoMode {
		demoConfirm := envOrDefault("LABTETHER_DEMO_CONFIRM", "")
		if demoConfirm != "yes-this-is-a-demo-instance" {
			return newStartupFailure(startupFailureDemoConfiguration, errors.New("demo mode confirmation is not configured"))
		}
		log.Println("WARNING: DEMO MODE ENABLED — this instance is public, read-only, and unauthenticated. Do not use for production.")
	}

	registry := buildConnectorRegistry()

	// Policy state (inline from policy service).
	policyCfg := loadPolicyConfigFromEnv()
	policyState := newPolicyRuntimeState(policyCfg)
	go refreshPolicyRuntimeSettingsDirect(ctx, pgStore, policyState)
	go refreshSecurityRuntimeSettingsDirect(ctx, pgStore)

	// Bootstrap admin user for MVP single-user auth.
	if !demoMode {
		if err := bootstrapAdminUser(pgStore); err != nil {
			return newStartupFailure(startupFailureAdminBootstrap, err)
		}
	}

	oidcProvider, oidcAutoProvision, err := loadOIDCProviderFromEnv(ctx)
	if err != nil {
		log.Printf("labtether auth: warning: oidc initialization failed: %v (starting with oidc disabled)", err)
	}
	oidcRef := authpkg.NewOIDCProviderRef(oidcProvider, oidcAutoProvision)

	srv := newAPIServer(pgStore, secretsManager, policyState, registry, authValidator, oidcRef, newInstallStateStore(dataDir))
	srv.dataDir = dataDir
	srv.demoMode = demoMode

	if demoMode {
		srv.demoRateLimiter = newDemoSessionRateLimiter(10, time.Minute)
		if err := srv.bootstrapDemoUser(); err != nil {
			return newStartupFailure(startupFailureDemoBootstrap, err)
		}
		go demo.RunKeepalive(ctx, pgStore.Pool())
	}

	// Derive TOTP encryption key for 2FA secret storage.
	totpKey, err := deriveTOTPKey(runtimeSecrets.EncryptionKey)
	if err != nil {
		return newStartupFailure(startupFailureTOTPKey, err)
	}
	srv.totpEncryptionKey = totpKey

	configureServerRuntime(ctx, srv, registry, secretsManager, pgStore)
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

	runtimeCtx, stopRuntime := context.WithCancel(ctx)
	collectorRuntime := srv.ensureCollectorsDeps()
	collectorRuntime.CollectorRuntimeContext = runtimeCtx
	defer func() {
		runtimeDrained = finalizeHubRuntimeDrain(
			srv,
			stopRuntime,
			hubRuntimeShutdownTimeout,
			httpConnectionsDrained,
		)
		if runtimeDrained {
			log.Printf("labtether: runtime shutdown complete")
		}
	}()
	worker := initializeWorkerSubsystem(runtimeCtx, srv, pgStore)
	startRuntimeLoops(runtimeCtx, srv, pgStore, worker.state, worker.retentionTracker)

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
	builtInSANHosts := builtInCertificateSANHosts(srv.externalURL)
	switch defaultTLSMode {
	case "disabled":
		port = httpPort
		log.Printf("labtether: TLS disabled — serving HTTP on :%s", port)

	case "external":
		if tlsCertFile == "" || tlsKeyFile == "" {
			return newStartupFailure(startupFailureTLSConfiguration, errors.New("external TLS certificate or key path is not configured"))
		}
		port = httpsPort
		srv.tlsState.Enabled = true
		log.Printf("labtether: TLS enabled (external cert=%s, key=%s) — serving HTTPS on :%s", tlsCertFile, tlsKeyFile, port)

	default: // "auto"
		certsDir := dataDir + "/certs"

		// Always provision the built-in self-signed CA so LAN-only agents
		// can still pin it, regardless of which cert the HTTPS listener uses.
		builtInResult, builtInErr := certmgr.Provision(certsDir, builtInSANHosts...)
		if builtInErr != nil {
			return newStartupFailure(startupFailureTLSConfiguration, builtInErr)
		}
		srv.tlsState.CACertPEM = builtInResult.CACertPEM
		srv.tlsState.CertReloader = builtInResult.Reloader
		go builtInResult.Reloader.Run(ctx)
		shareCACert(builtInResult.CACertPEM, "/ca")

		// Prefer Tailscale cert (publicly trusted) over self-signed.
		if tsCertPath, tsKeyPath, tsDomain, tsErr := provisionTailscaleCert(certsDir); tsErr == nil && shouldUseTailscaleCertificate(tsDomain, builtInSANHosts) {
			tlsCertFile = tsCertPath
			tlsKeyFile = tsKeyPath
			defaultTLSSource = tlsSourceTailscale
			tsReloader := newTailscaleCertReloader(tsCertPath, tsKeyPath, tsDomain, certsDir)
			srv.tlsState.TailscaleCertReloader = tsReloader
			go tsReloader.Run(ctx)
			log.Printf("labtether: TLS using Tailscale cert for %s — serving HTTPS on :%s", tsDomain, httpsPort)
		} else {
			if tsErr == nil {
				log.Printf("labtether: Tailscale cert for %s does not cover configured external host %s, using built-in self-signed", tsDomain, builtInSANHosts[0])
			} else {
				log.Printf("labtether: Tailscale cert not available (%v), using built-in self-signed", tsErr)
			}
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
		return newStartupFailure(startupFailureTLSConfiguration, err)
	} else {
		srv.tlsState.HttpsPort = v
	}
	if v, err := strconv.Atoi(httpPort); err != nil {
		return newStartupFailure(startupFailureTLSConfiguration, err)
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
				return newStartupFailure(startupFailureTLSConfiguration, err)
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
		if portErr == nil {
			portErr = errors.New("port is outside the valid range")
		}
		return newStartupFailure(startupFailureTLSConfiguration, portErr)
	}
	if strings.TrimSpace(srv.externalURL) != "" {
		if _, ok := srv.sanitizedExternalHubURL(); !ok {
			log.Printf("labtether warning: ignoring invalid or insecure LABTETHER_EXTERNAL_URL=%q", srv.externalURL)
		}
	}
	redirectHost := ""
	if externalURL, ok := srv.sanitizedExternalHubURL(); ok {
		if parsed, parseErr := url.Parse(externalURL); parseErr == nil {
			redirectHost = parsed.Hostname()
		}
	}
	if redirectHost == "" && isLoopbackHubBindAddress(bindAddress) {
		// Direct development runs are loopback-only, so the bind address is a
		// safe fallback when no public external URL was configured.
		redirectHost = bindAddress
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
			return newStartupFailure(startupFailureTLSConfiguration, err)
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

	// Wrap every handler with audit logging middleware. This runs after CORS
	// (applied next) but before gzip so it captures the real response status
	// code. It logs method, path, status, duration, and authenticated actor
	// for every API request and appends best-effort audit events to the store.
	for path, h := range handlers {
		wrapped := srv.auditMiddleware(http.HandlerFunc(h))
		handlers[path] = wrapped.ServeHTTP
	}

	// Wrap every handler with CORS middleware. This runs as the outermost
	// layer so that OPTIONS preflight requests receive proper CORS headers
	// and a 204 response without passing through auth middleware.
	for path, h := range handlers {
		wrapped := srv.corsMiddleware(http.HandlerFunc(h))
		handlers[path] = wrapped.ServeHTTP
	}

	// In demo mode, wrap every handler with read-only middleware as the
	// outermost layer. This blocks all mutating requests (POST/PUT/DELETE/PATCH)
	// except for explicitly allowlisted paths like /api/demo/session.
	if srv.demoMode {
		for path, h := range handlers {
			finalHandler := demoReadOnlyMiddleware(http.HandlerFunc(h))
			handlers[path] = finalHandler.ServeHTTP
		}
	}

	// Wrap every handler with panic recovery as the absolute outermost layer.
	for path, h := range handlers {
		wrapped := servicehttp.RecoverMiddleware(http.HandlerFunc(h))
		handlers[path] = wrapped.ServeHTTP
	}

	httpCfg := servicehttp.Config{
		Name:             "labtether",
		Version:          version,
		Port:             port,
		BindAddress:      bindAddress,
		TLSCertFile:      tlsCertFile,
		TLSKeyFile:       tlsKeyFile,
		RedirectHTTPPort: redirectPort,
		HTTPSPort:        httpsPortInt,
		RedirectHost:     redirectHost,
		ExtraHandlers:    handlers,
		DBPool:           pgStore.Pool(),
		ReadinessCheck: func() error {
			if err := pgStore.Pool().Ping(ctx); err != nil {
				return fmt.Errorf("database: %w", err)
			}
			return nil
		},
	}
	if srv.tlsState.CertSwitcher != nil {
		httpCfg.GetCertificate = srv.tlsState.CertSwitcher.GetCertificate
	}

	runErr := servicehttp.Run(ctx, httpCfg)
	if errors.Is(runErr, servicehttp.ErrHTTPDrainIncomplete) {
		httpConnectionsDrained = false
	}
	if cause := context.Cause(ctx); errors.Is(cause, persistence.ErrHubRuntimeLeaseLost) {
		if runErr != nil {
			cause = errors.Join(cause, runErr)
		}
		return newStartupFailure(startupFailureRuntimeOwnership, cause)
	}
	if runErr != nil {
		return newStartupFailure(startupFailureServerRuntime, runErr)
	}
	return nil
}
