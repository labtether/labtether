package resources

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/edges"
	"github.com/labtether/labtether/internal/fileproto"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/terminal"
)

// AuditEvent is a type alias for audit.Event so callers don't need to import
// the audit package just to satisfy the Deps interface.
type AuditEvent = audit.Event

// Deps holds all dependencies required by the resources handler package.
// This package contains agent-bridge handlers (file, process, service, disk,
// network, package, cron, users, journal), telemetry ingest handlers
// (frontend perf, mobile client), and extracted CRUD/query handlers
// (metrics, dependencies, edges, WoL, groups, group profiles, failover pairs,
// credentials, synthetic checks, link suggestions, device registration,
// restart, asset terminal config).
type Deps struct {
	// Agent manager
	AgentMgr *agentmgr.AgentManager

	// Core stores.
	AssetStore          persistence.AssetStore
	GroupStore          persistence.GroupStore
	GroupProfileStore   persistence.GroupProfileStore
	FailoverStore       persistence.FailoverStore
	TelemetryStore      persistence.TelemetryStore
	LogStore            persistence.LogStore
	DependencyStore     persistence.DependencyStore
	EdgeStore           edges.Store
	SyntheticStore      persistence.SyntheticStore
	LinkSuggestionStore persistence.LinkSuggestionStore
	CredentialStore     persistence.CredentialStore
	AuditStore          persistence.AuditStore
	RetentionStore      persistence.RetentionStore
	DB                  PushDeviceStore

	// File protocol connection pool and stores.
	FileProtoPool       *fileproto.Pool
	FileConnectionStore persistence.FileConnectionStore
	FileTransferStore   persistence.FileTransferStore
	ActiveTransfers     *sync.Map // transfer ID → context.CancelFunc

	// Remote desktop bookmark store.
	RemoteBookmarkStore RemoteBookmarkStoreInterface

	// ManualDeviceDB is used by HandleManualDeviceRoutes to set host and
	// transport_type on manually-created assets. May be nil in deployments
	// that skip raw SQL (e.g. tests).
	ManualDeviceDB ManualDeviceExecer

	// Agent bridge maps (shared with cmd/labtether for WS handler dispatch).
	FileBridges    *sync.Map
	ProcessBridges *sync.Map
	ServiceBridges *sync.Map
	JournalBridges *sync.Map
	DiskBridges    *sync.Map
	NetworkBridges *sync.Map
	PackageBridges *sync.Map
	CronBridges    *sync.Map
	UsersBridges   *sync.Map

	// Auth middleware injected from cmd/labtether.
	WrapAuth func(http.HandlerFunc) http.HandlerFunc

	// Stores for heartbeat and delete cascade logic.
	EnrollmentStore   persistence.EnrollmentStore
	HubCollectorStore persistence.HubCollectorStore

	// Coordinator removal callbacks (nil-safe in handlers).
	RemoveDockerHost     func(assetID string)
	RemoveWebServiceHost func(assetID string)

	// SendSSHKeyRemoveToAsset sends the ssh_key_remove message to the agent
	// currently connected for the given asset. It is a no-op when the asset
	// is not connected or hub identity is not set.
	SendSSHKeyRemoveToAsset func(assetID string)

	// PersistCanonicalHeartbeat is called after every successful heartbeat
	// upsert to update the canonical model. May be nil in deployments that
	// skip canonical model writes (e.g. unit tests).
	PersistCanonicalHeartbeatFn func(assetEntry assets.Asset, req assets.HeartbeatRequest)

	// CollectorConfigString extracts a string value from a collector config
	// map. Injected from cmd/labtether to avoid an import cycle with the
	// collectors package.
	CollectorConfigString func(config map[string]any, key string) string

	// NormalizeAssetKey returns the normalised lowercase-alphanum form of an
	// asset ID used for legacy docker child ID pattern matching.
	NormalizeAssetKey func(value string) string

	// AutoDockerCollectorAssetID returns the stable cluster asset ID for an
	// auto-provisioned Docker collector rooted at the given agent asset.
	AutoDockerCollectorAssetID func(agentAssetID string) string

	// Asset sub-handler callbacks injected from cmd/labtether for dispatch
	// targets that have not yet been extracted into this package.
	// All fields are optional (nil-safe guards in HandleAssetActions).
	HandleDesktopCredentials         http.HandlerFunc
	HandleRetrieveDesktopCredentials http.HandlerFunc
	HandleDisplayList                func(w http.ResponseWriter, r *http.Request, assetID string)
	HandlePushHubKey                 func(w http.ResponseWriter, r *http.Request, assetID string)
	HandleTestProtocolConnection     func(w http.ResponseWriter, r *http.Request, assetID, protocol string)
	HandleListProtocolConfigs        func(w http.ResponseWriter, r *http.Request, assetID string)
	HandleCreateProtocolConfig       func(w http.ResponseWriter, r *http.Request, assetID string)
	HandleUpdateProtocolConfig       func(w http.ResponseWriter, r *http.Request, assetID, protocol string)
	HandleDeleteProtocolConfig       func(w http.ResponseWriter, r *http.Request, assetID, protocol string)

	// Cross-cutting helpers injected from cmd/labtether.
	DecodeJSONBody  func(w http.ResponseWriter, r *http.Request, dst any) error
	ExecuteViaAgent func(job terminal.CommandJob) terminal.CommandResult

	// EnforceRateLimit returns false (and writes 429) if the rate limit has
	// been exceeded for the given bucket.
	EnforceRateLimit func(w http.ResponseWriter, r *http.Request, bucket string, limit int, window time.Duration) bool

	// PrincipalActorID extracts the actor ID from the request context.
	PrincipalActorID func(ctx context.Context) string

	// UserIDFromContext extracts the user ID from the request context.
	UserIDFromContext func(ctx context.Context) string

	// SecretsManager for credential encryption/decryption.
	SecretsManager SecretsManagerInterface

	// AppendAuditEventBestEffort appends an audit event, logging on failure.
	AppendAuditEventBestEffort func(event AuditEvent, logMessage string)
}

// PushDeviceStore is a minimal interface for push device registration.
type PushDeviceStore interface {
	UpsertPushDevice(ctx context.Context, device persistence.PushDevice) error
	DeletePushDevice(ctx context.Context, userID, deviceID string) error
}

// ManualDeviceExecer is a narrow interface for raw SQL execution used only by
// HandleManualDeviceRoutes. It is satisfied by *pgxpool.Pool (which returns
// pgconn.CommandTag, compatible with the error-only check pattern used here).
type ManualDeviceExecer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// SecretsManagerInterface is the subset of secrets.Manager used by credential handlers.
type SecretsManagerInterface interface {
	EncryptString(plaintext, aad string) (string, error)
	DecryptString(ciphertext, aad string) (string, error)
}

// RemoteBookmarkStoreInterface is the subset of persistence.RemoteBookmarkStore
// used by remote bookmark handlers.
type RemoteBookmarkStoreInterface interface {
	ListRemoteBookmarks(ctx context.Context) ([]persistence.RemoteBookmark, error)
	GetRemoteBookmark(ctx context.Context, id string) (*persistence.RemoteBookmark, error)
	CreateRemoteBookmark(ctx context.Context, bm *persistence.RemoteBookmark) error
	UpdateRemoteBookmark(ctx context.Context, bm persistence.RemoteBookmark) error
	DeleteRemoteBookmark(ctx context.Context, id string) error
}

// RegisterRoutes registers all resource-related HTTP routes on the given handler map.
func RegisterRoutes(handlers map[string]http.HandlerFunc, d *Deps) {
	handlers["/files/"] = d.WrapAuth(d.HandleFiles)
	handlers["/processes/"] = d.WrapAuth(d.HandleProcesses)
	handlers["/services/"] = d.WrapAuth(d.HandleServices)
	handlers["/disks/"] = d.WrapAuth(d.HandleDisks)
	handlers["/network/"] = d.WrapAuth(d.HandleNetworks)
	handlers["/packages/"] = d.WrapAuth(d.HandlePackages)
	handlers["/cron/"] = d.WrapAuth(d.HandleCrons)
	handlers["/users/"] = d.WrapAuth(d.HandleUsers)
	handlers["/logs/journal/"] = d.WrapAuth(d.HandleJournalLogs)
	handlers["/telemetry/frontend/perf"] = d.WrapAuth(d.HandleFrontendPerfTelemetry)
	handlers["/telemetry/mobile/client"] = d.WrapAuth(d.HandleMobileClientTelemetry)
	handlers["/metrics/overview"] = d.WrapAuth(d.HandleMetricsOverview)
	handlers["/metrics/assets/"] = d.WrapAuth(d.HandleAssetMetrics)
	handlers["/dependencies"] = d.WrapAuth(d.HandleDependencies)
	handlers["/dependencies/"] = d.WrapAuth(d.HandleDependencyActions)
	handlers["/edges"] = d.WrapAuth(d.HandleEdges)
	handlers["/edges/"] = d.WrapAuth(d.HandleEdgeByID)
	handlers["/composites"] = d.WrapAuth(d.HandleComposites)
	handlers["/composites/"] = d.WrapAuth(d.HandleCompositeActions)
	handlers["/groups"] = d.WrapAuth(d.HandleGroups)
	handlers["/groups/"] = d.WrapAuth(d.HandleGroupActions)
	handlers["/group-profiles"] = d.WrapAuth(d.HandleGroupProfiles)
	handlers["/group-profiles/"] = d.WrapAuth(d.HandleGroupProfileActions)
	handlers["/group-failover-pairs"] = d.WrapAuth(d.HandleFailoverPairs)
	handlers["/group-failover-pairs/"] = d.WrapAuth(d.HandleFailoverPairActions)
	handlers["/synthetic-checks"] = d.WrapAuth(d.HandleSyntheticChecks)
	handlers["/synthetic-checks/"] = d.WrapAuth(d.HandleSyntheticCheckActions)
	handlers["/links/suggestions"] = d.WrapAuth(d.HandleLinkSuggestions)
	handlers["/links/suggestions/"] = d.WrapAuth(d.HandleLinkSuggestionActions)
	handlers["/links/manual"] = d.WrapAuth(d.HandleManualLink)
	handlers["/api/v1/devices/register"] = d.WrapAuth(d.HandleDeviceRegister)
	handlers["/settings/retention"] = d.WrapAuth(d.HandleRetentionSettings)
	handlers["/audit/events"] = d.WrapAuth(d.HandleAuditEvents)
	handlers["/discovery/proposals"] = d.WrapAuth(d.HandleProposals)
	handlers["/discovery/proposals/"] = d.WrapAuth(d.HandleProposalActions)
}

// RegisterWSHandlers registers WebSocket message handlers for resource-related
// agent messages into the shared router.
func RegisterWSHandlers(router map[string]func(*agentmgr.AgentConn, agentmgr.Message), d *Deps) {
	router[agentmgr.MsgFileListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentFileListed(conn, msg)
	}
	router[agentmgr.MsgFileData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentFileData(conn, msg)
	}
	router[agentmgr.MsgFileWritten] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentFileWritten(conn, msg)
	}
	router[agentmgr.MsgFileResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentFileResult(conn, msg)
	}
	router[agentmgr.MsgProcessListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentProcessListed(conn, msg)
	}
	router[agentmgr.MsgProcessKillResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentProcessKillResult(conn, msg)
	}
	router[agentmgr.MsgServiceListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentServiceListed(conn, msg)
	}
	router[agentmgr.MsgServiceResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentServiceResult(conn, msg)
	}
	router[agentmgr.MsgDiskListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentDiskListed(conn, msg)
	}
	router[agentmgr.MsgNetworkListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentNetworkListed(conn, msg)
	}
	router[agentmgr.MsgNetworkResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentNetworkResult(conn, msg)
	}
	router[agentmgr.MsgPackageListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentPackageListed(conn, msg)
	}
	router[agentmgr.MsgPackageResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentPackageResult(conn, msg)
	}
	router[agentmgr.MsgCronListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentCronListed(conn, msg)
	}
	router[agentmgr.MsgUsersListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentUsersListed(conn, msg)
	}
	router[agentmgr.MsgJournalEntries] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentJournalEntries(conn, msg)
	}
	router[agentmgr.MsgWoLResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentWoLResult(conn, msg)
	}
}
