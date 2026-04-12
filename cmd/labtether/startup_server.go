package main

import (
	"context"
	"log"
	"path/filepath"
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/connectors/docker"
	"github.com/labtether/labtether/internal/connectors/homeassistant"
	"github.com/labtether/labtether/internal/connectors/pbs"
	"github.com/labtether/labtether/internal/connectors/portainer"
	"github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/connectors/webservice"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/discovery"
	"github.com/labtether/labtether/internal/fileproto"
	agentspkg "github.com/labtether/labtether/internal/hubapi/agents"
	authpkg "github.com/labtether/labtether/internal/hubapi/auth"
	"github.com/labtether/labtether/internal/installstate"
	"github.com/labtether/labtether/internal/modelmap"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/secrets"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/labtether/labtether/internal/topology"
)

func loadSecretsManager(encodedKey string) (*secrets.Manager, error) {
	encodedKey = strings.TrimSpace(encodedKey)
	if encodedKey == "" {
		return nil, nil
	}
	return secrets.NewManagerFromEncodedKey(encodedKey)
}

func newInstallStateStore(dataDir string) *installstate.Store {
	return installstate.New(filepath.Join(strings.TrimSpace(dataDir), "install"))
}

func buildConnectorRegistry() *connectorsdk.Registry {
	registry := connectorsdk.NewRegistry()
	registry.Register(modelmap.WrapConnector(proxmox.New()))
	registry.Register(modelmap.WrapConnector(pbs.New()))
	registry.Register(modelmap.WrapConnector(homeassistant.New()))
	registry.Register(modelmap.WrapConnector(truenas.New()))
	registry.Register(modelmap.WrapConnector(portainer.New()))
	return registry
}

func defaultNotificationAdapters() map[string]notifications.Adapter {
	return map[string]notifications.Adapter{
		notifications.ChannelTypeWebhook: &notifications.WebhookAdapter{},
		notifications.ChannelTypeEmail:   &notifications.EmailAdapter{},
		notifications.ChannelTypeSlack:   &notifications.SlackAdapter{},
		notifications.ChannelTypeAPNs:    &notifications.APNsAdapter{},
		notifications.ChannelTypeNtfy:    &notifications.NtfyAdapter{},
		notifications.ChannelTypeGotify:  &notifications.GotifyAdapter{},
	}
}

func newAPIServer(
	pgStore *persistence.PostgresStore,
	secretsManager *secrets.Manager,
	policyState *policyRuntimeState,
	registry *connectorsdk.Registry,
	authValidator *auth.TokenValidator,
	oidcRef *authpkg.OIDCProviderRef,
	installStateStore *installstate.Store,
) *apiServer {
	srv := &apiServer{
		db:                      pgStore,
		terminalStore:           pgStore,
		terminalPersistentStore: pgStore,
		terminalBookmarkStore:   pgStore,
		terminalScrollbackStore: pgStore,
		terminalInMemStore:      terminal.NewStore(),
		auditStore:              pgStore,
		assetStore:              pgStore,
		groupStore:              pgStore,
		groupMaintenanceStore:   pgStore,
		groupProfileStore:       pgStore,
		failoverStore:           pgStore,
		credentialStore:         pgStore,
		canonicalStore:          pgStore,
		telemetryStore:          pgStore,
		logStore:                pgStore,
		actionStore:             pgStore,
		updateStore:             pgStore,
		alertStore:              pgStore,
		alertInstanceStore:      pgStore,
		incidentStore:           pgStore,
		retentionStore:          pgStore,
		runtimeStore:            pgStore,
		authStore:               pgStore,
		notificationStore:       pgStore,
		dependencyStore:         pgStore,
		syntheticStore:          pgStore,
		incidentEventStore:      pgStore,
		hubCollectorStore:       pgStore,
		enrollmentStore:         pgStore,
		adminResetStore:         pgStore,
		presenceStore:           pgStore,
		apiKeyStore:             pgStore,
		scheduleStore:           pgStore,
		webhookStore:            pgStore,
		savedActionStore:        pgStore,
		edgeStore:               pgStore,
		topologyStore:           topology.NewPostgresStore(pgStore.Pool()),
		linkSuggestionStore:     pgStore,
		secretsManager:          secretsManager,
		policyState:             policyState,
		connectorRegistry:       registry,
		authValidator:           authValidator,
		oidcRef:                 oidcRef,
		agentMgr:                agentmgr.NewManager(),
		broadcaster:             newEventBroadcaster(),
		notificationDispatcher: NotificationDispatcher{
			Adapters:    defaultNotificationAdapters(),
			DispatchSem: make(chan struct{}, 32),
		},
		collectorDispatchSem: make(chan struct{}, 8),
		rateLimiter:          RateLimiter{Windows: make(map[string]rateCounter, 64)},
		streamTicketStore:    StreamTicketStore{Tickets: make(map[string]streamTicket, 128)},
		desktopSessionOpts:   make(map[string]desktopSessionOptions, 128),
		desktopSPICE:         make(map[string]desktopSPICEProxyTarget, 128),
		agentCache: func() *agentspkg.AgentCache {
			cache := &agentspkg.AgentCache{
				RuntimeDir: envOrDefault("LABTETHER_AGENT_CACHE_DIR", "/data/agents"),
				BakedInDir: envOrDefault("LABTETHER_AGENT_DIR", "/opt/labtether/agents"),
			}
			if err := cache.LoadManifest(); err != nil {
				log.Printf("WARN: agent manifest not found: %v (agent binary endpoints will return 503 until manifest is available)", err)
			}
			return cache
		}(),
		externalURL:       strings.TrimRight(strings.TrimSpace(envOrDefault("LABTETHER_EXTERNAL_URL", "")), "/"),
		pendingAgents:     newPendingAgents(),
		challengeStore:    auth.NewChallengeStore(),
		installStateStore: installStateStore,
		fileProtoPool:     fileproto.NewPool(),
		apiKeyTouchCh:     make(chan string, 100),
	}
	// Wire broadcaster to bump status aggregate generation on every mutation event.
	srv.broadcaster.SetOnBroadcast(func() { srv.statusCache.Generation.Add(1) })
	srv.proxmoxDeps = srv.buildProxmoxDeps()
	return srv
}

// startMDNSAdvertiser creates and starts a mDNS/Bonjour DNS-SD advertiser
// for the LabTether hub so that iOS companion apps can discover it on the
// local network via NWBrowser with the "_labtether._tcp" service type.
//
// Errors are non-fatal: mDNS is a best-effort discovery mechanism and the
// hub operates normally without it.
func startMDNSAdvertiser(ctx context.Context, port int) {
	version := envOrDefault("APP_VERSION", "dev")
	advertiser, err := discovery.NewMDNSAdvertiser(port, version)
	if err != nil {
		log.Printf("labtether: mDNS advertiser init failed (discovery disabled): %v", err)
		return
	}
	if err := advertiser.Start(); err != nil {
		log.Printf("labtether: mDNS advertiser start failed (discovery disabled): %v", err)
		return
	}
	go func() {
		<-ctx.Done()
		advertiser.Stop()
	}()
}

func configureServerRuntime(srv *apiServer, registry *connectorsdk.Registry, secretsManager *secrets.Manager, pgStore *persistence.PostgresStore) {
	// Docker coordinator — must be created after srv so agentMgr is available.
	srv.dockerCoordinator = docker.NewCoordinator(srv.agentMgr)
	registry.Register(modelmap.WrapConnector(srv.dockerCoordinator))

	// Web service coordinator — aggregates web service discovery reports from agents.
	srv.webServiceCoordinator = webservice.NewCoordinator(pgStore)

	// Hub SSH identity for agent key auto-provisioning.
	if secretsManager != nil {
		identity, identityErr := ensureHubSSHIdentity(srv)
		if identityErr != nil {
			log.Printf("labtether warning: hub SSH identity not available: %v", identityErr)
		} else {
			srv.hubIdentity = identity
		}
	}
}
