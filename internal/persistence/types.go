package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/apikeys"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/dependencies"
	"github.com/labtether/labtether/internal/enrollment"
	"github.com/labtether/labtether/internal/groupfailover"
	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/groupprofiles"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/retention"
	"github.com/labtether/labtether/internal/savedactions"
	"github.com/labtether/labtether/internal/schedules"
	"github.com/labtether/labtether/internal/synthetic"
	"github.com/labtether/labtether/internal/telemetry"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/labtether/labtether/internal/updates"
	"github.com/labtether/labtether/internal/webhooks"
)

// ErrNotFound is returned when a requested resource does not exist.
var ErrNotFound = errors.New("not found")

// ReliabilityRecord stores a historical group reliability score snapshot.
type ReliabilityRecord struct {
	ID          string         `json:"id"`
	GroupID     string         `json:"group_id"`
	Score       int            `json:"score"`
	Grade       string         `json:"grade"`
	Factors     map[string]any `json:"factors,omitempty"`
	WindowHours int            `json:"window_hours"`
	ComputedAt  time.Time      `json:"computed_at"`
}

// TerminalStore provides persistence for terminal sessions and commands.
type TerminalStore interface {
	CreateSession(req terminal.CreateSessionRequest) (terminal.Session, error)
	GetSession(id string) (terminal.Session, bool, error)
	UpdateSession(session terminal.Session) error
	ListSessions() ([]terminal.Session, error)
	DeleteTerminalSession(id string) error
	AddCommand(sessionID string, req terminal.CreateCommandRequest, target, mode string) (terminal.Command, error)
	UpdateCommandResult(sessionID, commandID, status, output string) error
	ListCommands(sessionID string) ([]terminal.Command, error)
	ListRecentCommands(limit int) ([]terminal.Command, error)
}

// TerminalPersistentSessionStore provides persistence for durable terminal
// workspaces that can be reattached across client disconnects.
type TerminalPersistentSessionStore interface {
	CreateOrUpdatePersistentSession(req terminal.CreatePersistentSessionRequest) (terminal.PersistentSession, error)
	GetPersistentSession(id string) (terminal.PersistentSession, bool, error)
	ListPersistentSessions() ([]terminal.PersistentSession, error)
	UpdatePersistentSession(id string, req terminal.UpdatePersistentSessionRequest) (terminal.PersistentSession, error)
	MarkPersistentSessionAttached(id string, attachedAt time.Time) (terminal.PersistentSession, error)
	MarkPersistentSessionDetached(id string, detachedAt time.Time) (terminal.PersistentSession, error)
	DeletePersistentSession(id string) error
	MarkPersistentSessionArchived(id string, archivedAt time.Time) (terminal.PersistentSession, error)
	ListDetachedOlderThan(threshold time.Time) ([]terminal.PersistentSession, error)
	ListAttachedSessions() ([]terminal.PersistentSession, error)
	MarkAllAttachedAsDetached() error
}

// TerminalBookmarkStore provides persistence for saved terminal connection bookmarks.
type TerminalBookmarkStore interface {
	CreateBookmark(req terminal.CreateBookmarkRequest) (terminal.Bookmark, error)
	GetBookmark(id string) (terminal.Bookmark, bool, error)
	ListBookmarks(actorID string) ([]terminal.Bookmark, error)
	UpdateBookmark(id string, req terminal.UpdateBookmarkRequest) (terminal.Bookmark, error)
	DeleteBookmark(id string) error
	TouchBookmarkLastUsed(id string, at time.Time) error
}

// TerminalScrollbackStore provides persistence for terminal session scrollback buffers.
type TerminalScrollbackStore interface {
	UpsertScrollback(persistentSessionID string, buffer []byte, bufferSize int, totalLines int) error
	GetScrollback(persistentSessionID string) ([]byte, error)
	DeleteScrollback(persistentSessionID string) error
}

// AuditStore provides persistence for audit events.
type AuditStore interface {
	Append(event audit.Event) error
	List(limit, offset int) ([]audit.Event, error)
}

// AssetStore provides persistence for asset inventory and heartbeat updates.
type AssetStore interface {
	UpsertAssetHeartbeat(req assets.HeartbeatRequest) (assets.Asset, error)
	UpdateAsset(id string, req assets.UpdateRequest) (assets.Asset, error)
	ListAssets() ([]assets.Asset, error)
	GetAsset(id string) (assets.Asset, bool, error)
	DeleteAsset(id string) error
}

// GroupAssetStore is an optional optimization interface for loading the assets
// attached to a single group without scanning the full inventory.
type GroupAssetStore interface {
	ListAssetsByGroup(groupID string) ([]assets.Asset, error)
}

// GroupStore provides persistence for hierarchical asset groups.
type GroupStore interface {
	CreateGroup(req groups.CreateRequest) (groups.Group, error)
	UpdateGroup(id string, req groups.UpdateRequest) (groups.Group, error)
	GetGroup(id string) (groups.Group, bool, error)
	ListGroups() ([]groups.Group, error)
	GetGroupTree() ([]groups.TreeNode, error)
	DeleteGroup(id string) error
	IsAncestor(candidateAncestorID, descendantID string) (bool, error)
}

// TelemetryStore provides canonical metrics persistence and query paths.
type TelemetryStore interface {
	AppendSamples(ctx context.Context, samples []telemetry.MetricSample) error
	Snapshot(assetID string, at time.Time) (telemetry.Snapshot, error)
	Series(assetID string, start, end time.Time, step time.Duration) ([]telemetry.Series, error)
}

// TelemetrySnapshotBatchStore is an optional optimization interface for
// loading latest metric snapshots for many assets in a single store call.
type TelemetrySnapshotBatchStore interface {
	SnapshotMany(assetIDs []string, at time.Time) (map[string]telemetry.Snapshot, error)
}

// TelemetryDynamicStore is an optional interface for stores that can return
// dynamic (all-metric) snapshots. Callers that type-assert to this interface
// receive a full map of every metric for an asset, not just the 6 canonical ones.
type TelemetryDynamicStore interface {
	DynamicSnapshotForAsset(assetID string, at time.Time) (telemetry.DynamicSnapshot, error)
	DynamicSnapshotMany(assetIDs []string, at time.Time) (map[string]telemetry.DynamicSnapshot, error)
}

// TelemetryAlertBatchStore is an optional optimization interface for alert
// evaluation paths that only need one metric or simple sample presence checks.
type TelemetryAlertBatchStore interface {
	MetricSeriesBatch(assetIDs []string, metric string, start, end time.Time, step time.Duration) (map[string]telemetry.Series, error)
	AssetsWithSamples(assetIDs []string, start, end time.Time) (map[string]bool, error)
}

// LogStore provides normalized log/event persistence and query paths.
type LogStore interface {
	AppendEvent(event logs.Event) error
	QueryEvents(req logs.QueryRequest) ([]logs.Event, error)
	ListSources(limit int) ([]logs.SourceSummary, error)
	SaveView(req logs.SavedViewRequest) (logs.SavedView, error)
	ListViews(limit int) ([]logs.SavedView, error)
	GetView(id string) (logs.SavedView, bool, error)
	UpdateView(id string, req logs.SavedViewRequest) (logs.SavedView, error)
	DeleteView(id string) error
}

// LogBatchAppendStore is an optional optimization interface for appending
// many log events in one store call.
type LogBatchAppendStore interface {
	AppendEvents(events []logs.Event) error
}

// DeadLetterLogStore is an optional optimization interface for dead-letter
// list/analytics query paths that do not require full log event payloads.
type DeadLetterLogStore interface {
	QueryDeadLetterEvents(from, to time.Time, limit int) ([]logs.DeadLetterEvent, error)
}

// DeadLetterLogCountStore is an optional optimization interface for obtaining
// exact dead-letter totals within a time range without fetching event payloads.
type DeadLetterLogCountStore interface {
	CountDeadLetterEvents(from, to time.Time) (int, error)
}

// ActionStore provides persistence for typed action runs.
type ActionStore interface {
	CreateActionRun(req actions.ExecuteRequest) (actions.Run, error)
	GetActionRun(id string) (actions.Run, bool, error)
	ListActionRuns(limit, offset int, runType, status string) ([]actions.Run, error)
	DeleteActionRun(id string) error
	ApplyActionResult(result actions.Result) error
}

// UpdateStore provides persistence for update planning and execution runs.
type UpdateStore interface {
	CreateUpdatePlan(req updates.CreatePlanRequest) (updates.Plan, error)
	ListUpdatePlans(limit int) ([]updates.Plan, error)
	GetUpdatePlan(id string) (updates.Plan, bool, error)
	DeleteUpdatePlan(id string) error
	CreateUpdateRun(plan updates.Plan, req updates.ExecutePlanRequest) (updates.Run, error)
	GetUpdateRun(id string) (updates.Run, bool, error)
	ListUpdateRuns(limit int, status string) ([]updates.Run, error)
	DeleteUpdateRun(id string) error
	ApplyUpdateResult(result updates.Result) error
}

// AlertRuleFilter defines query options for listing alert rules.
type AlertRuleFilter struct {
	Limit    int
	Offset   int
	Status   string
	Kind     string
	Severity string
}

// AlertStore provides persistence for alert rules and evaluation runs.
type AlertStore interface {
	CreateAlertRule(req alerts.CreateRuleRequest) (alerts.Rule, error)
	GetAlertRule(id string) (alerts.Rule, bool, error)
	ListAlertRules(filter AlertRuleFilter) ([]alerts.Rule, error)
	UpdateAlertRule(id string, req alerts.UpdateRuleRequest) (alerts.Rule, error)
	DeleteAlertRule(id string) error
	RecordAlertEvaluation(ruleID string, evaluation alerts.Evaluation) (alerts.Evaluation, error)
	ListAlertEvaluations(ruleID string, limit int) ([]alerts.Evaluation, error)
}

// IncidentFilter defines query options for listing incidents.
type IncidentFilter struct {
	Limit    int
	Offset   int
	Status   string
	Severity string
	GroupID  string
	Assignee string
	Source   string
}

// IncidentStore provides persistence for incidents and linked alerts.
type IncidentStore interface {
	CreateIncident(req incidents.CreateIncidentRequest) (incidents.Incident, error)
	GetIncident(id string) (incidents.Incident, bool, error)
	ListIncidents(filter IncidentFilter) ([]incidents.Incident, error)
	UpdateIncident(id string, req incidents.UpdateIncidentRequest) (incidents.Incident, error)
	DeleteIncident(id string) error
	LinkIncidentAlert(incidentID string, req incidents.LinkAlertRequest) (incidents.AlertLink, error)
	ListIncidentAlertLinks(incidentID string, limit int) ([]incidents.AlertLink, error)
	UnlinkIncidentAlert(incidentID, linkID string) error
}

// RetentionStore provides persistence for retention profile and pruning.
type RetentionStore interface {
	GetRetentionSettings() (retention.Settings, error)
	SaveRetentionSettings(settings retention.Settings) (retention.Settings, error)
	PruneExpiredData(now time.Time, settings retention.Settings) (retention.PruneResult, error)
}

// RuntimeSettingsStore provides persistence for UI runtime setting overrides.
type RuntimeSettingsStore interface {
	ListRuntimeSettingOverrides() (map[string]string, error)
	SaveRuntimeSettingOverrides(values map[string]string) (map[string]string, error)
	DeleteRuntimeSettingOverrides(keys []string) error
}

// CredentialStore provides encrypted profile inventory and per-asset terminal SSH mapping.
type CredentialStore interface {
	CreateCredentialProfile(profile credentials.Profile) (credentials.Profile, error)
	UpdateCredentialProfileSecret(id, secretCiphertext, passphraseCiphertext string, expiresAt *time.Time) (credentials.Profile, error)
	GetCredentialProfile(id string) (credentials.Profile, bool, error)
	ListCredentialProfiles(limit int) ([]credentials.Profile, error)
	MarkCredentialProfileUsed(id string, usedAt time.Time) error
	DeleteCredentialProfile(id string) error
	SaveAssetTerminalConfig(cfg credentials.AssetTerminalConfig) (credentials.AssetTerminalConfig, error)
	GetAssetTerminalConfig(assetID string) (credentials.AssetTerminalConfig, bool, error)
	DeleteAssetTerminalConfig(assetID string) error
	SaveDesktopConfig(cfg credentials.AssetDesktopConfig) (credentials.AssetDesktopConfig, error)
	GetDesktopConfig(assetID string) (credentials.AssetDesktopConfig, bool, error)
	DeleteDesktopConfig(assetID string) error
}

// AuthStore provides persistence for user accounts and sessions.
type AuthStore interface {
	GetUserByID(id string) (auth.User, bool, error)
	GetUserByUsername(username string) (auth.User, bool, error)
	GetUserByOIDCSubject(provider, subject string) (auth.User, bool, error)
	ListUsers(limit int) ([]auth.User, error)
	BootstrapFirstUser(username, passwordHash string) (auth.User, bool, error)
	CreateUser(username, passwordHash string) (auth.User, error)
	CreateUserWithRole(username, passwordHash, role, authProvider, oidcSubject string) (auth.User, error)
	UpdateUserPasswordHash(id, passwordHash string) error
	UpdateUserRole(id, role string) error
	DeleteUser(id string) error
	ListSessionsByUserID(userID string) ([]auth.Session, error)
	SetUserTOTPSecret(id, encryptedSecret string) error
	ConfirmUserTOTP(id, recoveryCodes string) error
	ClearUserTOTP(id string) error
	UpdateUserRecoveryCodes(id, recoveryCodes string) error
	ConsumeRecoveryCode(userID, code string) (bool, error)
	CreateAuthSession(userID, tokenHash string, expiresAt time.Time) (auth.Session, error)
	ValidateSession(tokenHash string) (auth.Session, bool, error)
	DeleteSession(id string) error
	DeleteSessionsByUserID(userID string) error
	DeleteExpiredSessions() (int64, error)
}

// APIKeyStore provides persistence for scoped API keys.
type APIKeyStore interface {
	CreateAPIKey(ctx context.Context, key apikeys.APIKey) error
	LookupAPIKeyByHash(ctx context.Context, secretHash string) (apikeys.APIKey, bool, error)
	GetAPIKey(ctx context.Context, id string) (apikeys.APIKey, bool, error)
	ListAPIKeys(ctx context.Context) ([]apikeys.APIKey, error)
	UpdateAPIKey(ctx context.Context, id string, name *string, scopes *[]string, allowedAssets *[]string, expiresAt *time.Time) error
	DeleteAPIKey(ctx context.Context, id string) error
	TouchAPIKeyLastUsed(ctx context.Context, id string) error
}

// AlertInstanceFilter defines query options for listing alert instances.
type AlertInstanceFilter struct {
	Limit    int
	Offset   int
	RuleID   string
	Status   string
	Severity string
}

// AlertInstanceStore provides persistence for alert instances and silences.
type AlertInstanceStore interface {
	CreateAlertInstance(req alerts.CreateInstanceRequest) (alerts.AlertInstance, error)
	GetAlertInstance(id string) (alerts.AlertInstance, bool, error)
	GetActiveInstanceByFingerprint(ruleID, fingerprint string) (alerts.AlertInstance, bool, error)
	ListAlertInstances(filter AlertInstanceFilter) ([]alerts.AlertInstance, error)
	UpdateAlertInstanceStatus(id, status string) (alerts.AlertInstance, error)
	UpdateAlertInstanceLastFired(id string) error
	DeleteAlertInstance(id string) error
	CreateAlertSilence(req alerts.CreateSilenceRequest) (alerts.AlertSilence, error)
	ListAlertSilences(limit int, activeOnly bool) ([]alerts.AlertSilence, error)
	GetAlertSilence(id string) (alerts.AlertSilence, bool, error)
	DeleteAlertSilence(id string) error
}

// NotificationStore provides persistence for notification channels, routes, and history.
type NotificationStore interface {
	CreateNotificationChannel(req notifications.CreateChannelRequest) (notifications.Channel, error)
	GetNotificationChannel(id string) (notifications.Channel, bool, error)
	ListNotificationChannels(limit int) ([]notifications.Channel, error)
	UpdateNotificationChannel(id string, req notifications.UpdateChannelRequest) (notifications.Channel, error)
	DeleteNotificationChannel(id string) error
	CreateAlertRoute(req notifications.CreateRouteRequest) (notifications.Route, error)
	GetAlertRoute(id string) (notifications.Route, bool, error)
	ListAlertRoutes(limit int) ([]notifications.Route, error)
	UpdateAlertRoute(id string, req notifications.UpdateRouteRequest) (notifications.Route, error)
	DeleteAlertRoute(id string) error
	CreateNotificationRecord(req notifications.CreateRecordRequest) (notifications.Record, error)
	ListNotificationHistory(limit int, channelID string) ([]notifications.Record, error)
	ListPendingRetries(ctx context.Context, now time.Time, limit int) ([]notifications.Record, error)
	UpdateRetryState(ctx context.Context, id string, retryCount int, nextRetryAt *time.Time, status string) error
}

// DependencyStore provides persistence for asset dependencies and blast-radius queries.
type DependencyStore interface {
	CreateAssetDependency(req dependencies.CreateDependencyRequest) (dependencies.Dependency, error)
	ListAssetDependencies(assetID string, limit int) ([]dependencies.Dependency, error)
	GetAssetDependency(id string) (dependencies.Dependency, bool, error)
	DeleteAssetDependency(id string) error
	BlastRadius(assetID string, maxDepth int) ([]dependencies.ImpactNode, error)
	UpstreamCauses(assetID string, maxDepth int) ([]dependencies.ImpactNode, error)
	LinkIncidentAsset(incidentID string, req incidents.LinkAssetRequest) (incidents.IncidentAsset, error)
	ListIncidentAssets(incidentID string, limit int) ([]incidents.IncidentAsset, error)
	UnlinkIncidentAsset(incidentID, linkID string) error
}

// SyntheticStore provides persistence for synthetic health checks and results.
type SyntheticStore interface {
	CreateSyntheticCheck(req synthetic.CreateCheckRequest) (synthetic.Check, error)
	GetSyntheticCheck(id string) (synthetic.Check, bool, error)
	GetSyntheticCheckByServiceID(ctx context.Context, serviceID string) (*synthetic.Check, error)
	ListSyntheticChecks(limit int, enabledOnly bool) ([]synthetic.Check, error)
	UpdateSyntheticCheck(id string, req synthetic.UpdateCheckRequest) (synthetic.Check, error)
	DeleteSyntheticCheck(id string) error
	RecordSyntheticResult(checkID string, result synthetic.Result) (synthetic.Result, error)
	ListSyntheticResults(checkID string, limit int) ([]synthetic.Result, error)
	UpdateSyntheticCheckStatus(id string, status string, runAt time.Time) error
}

// HubCollectorStore provides persistence for hub-initiated collectors.
type HubCollectorStore interface {
	CreateHubCollector(req hubcollector.CreateCollectorRequest) (hubcollector.Collector, error)
	GetHubCollector(id string) (hubcollector.Collector, bool, error)
	ListHubCollectors(limit int, enabledOnly bool) ([]hubcollector.Collector, error)
	UpdateHubCollector(id string, req hubcollector.UpdateCollectorRequest) (hubcollector.Collector, error)
	DeleteHubCollector(id string) error
	UpdateHubCollectorStatus(id, status, lastError string, collectedAt time.Time) error
}

// IncidentEventStore provides persistence for incident timeline events.
type IncidentEventStore interface {
	UpsertIncidentEvent(req incidents.CreateIncidentEventRequest) (incidents.IncidentEvent, error)
	ListIncidentEvents(incidentID string, limit int) ([]incidents.IncidentEvent, error)
}

// AdminResetStore provides database reset operations.
type AdminResetStore interface {
	ResetAllData() (AdminResetResult, error)
}

// AdminResetResult holds the outcome of a full data reset.
type AdminResetResult struct {
	TablesCleared int       `json:"tables_cleared"`
	ResetAt       time.Time `json:"reset_at"`
}

// EnrollmentStore provides persistence for enrollment tokens and per-agent API tokens.
type EnrollmentStore interface {
	CreateEnrollmentToken(tokenHash, label string, expiresAt time.Time, maxUses int) (enrollment.EnrollmentToken, error)
	ValidateEnrollmentToken(tokenHash string) (enrollment.EnrollmentToken, bool, error)
	ConsumeEnrollmentToken(tokenHash string) (enrollment.EnrollmentToken, bool, error)
	IncrementEnrollmentTokenUse(id string) error
	RevokeEnrollmentToken(id string) error
	ListEnrollmentTokens(limit int) ([]enrollment.EnrollmentToken, error)
	CreateAgentToken(assetID, tokenHash, enrolledVia string, expiresAt time.Time) (enrollment.AgentToken, error)
	ValidateAgentToken(tokenHash string) (enrollment.AgentToken, bool, error)
	TouchAgentTokenLastUsed(id string) error
	RevokeAgentToken(id string) error
	RevokeAgentTokensByAsset(assetID string) error
	ListAgentTokens(limit int) ([]enrollment.AgentToken, error)
	DeleteDeadTokens() (enrollmentDeleted int, agentDeleted int, err error)
}

// LinkSuggestion represents a proposed parent-child link between two assets.
type LinkSuggestion struct {
	ID            string     `json:"id"`
	SourceAssetID string     `json:"source_asset_id"`
	TargetAssetID string     `json:"target_asset_id"`
	MatchReason   string     `json:"match_reason"`
	Confidence    float64    `json:"confidence"`
	Status        string     `json:"status"` // pending, accepted, dismissed
	CreatedAt     time.Time  `json:"created_at"`
	ResolvedAt    *time.Time `json:"resolved_at,omitempty"`
	ResolvedBy    string     `json:"resolved_by,omitempty"`
}

// LinkSuggestionStore provides persistence for asset link suggestions.
type LinkSuggestionStore interface {
	CreateLinkSuggestion(sourceAssetID, targetAssetID, matchReason string, confidence float64) (LinkSuggestion, error)
	ListPendingLinkSuggestions() ([]LinkSuggestion, error)
	ResolveLinkSuggestion(id, status, resolvedBy string) error
}

// GroupMaintenanceStore provides persistence for group maintenance windows.
type GroupMaintenanceStore interface {
	CreateGroupMaintenanceWindow(groupID string, req groupmaintenance.CreateMaintenanceWindowRequest) (groupmaintenance.MaintenanceWindow, error)
	GetGroupMaintenanceWindow(groupID, windowID string) (groupmaintenance.MaintenanceWindow, bool, error)
	ListGroupMaintenanceWindows(groupID string, activeAt *time.Time, limit int) ([]groupmaintenance.MaintenanceWindow, error)
	UpdateGroupMaintenanceWindow(groupID, windowID string, req groupmaintenance.UpdateMaintenanceWindowRequest) (groupmaintenance.MaintenanceWindow, error)
	DeleteGroupMaintenanceWindow(groupID, windowID string) error
}

// GroupProfileStore provides persistence for group profile and drift workflows.
type GroupProfileStore interface {
	CreateGroupProfile(req groupprofiles.CreateProfileRequest) (groupprofiles.Profile, error)
	GetGroupProfile(id string) (groupprofiles.Profile, bool, error)
	ListGroupProfiles(limit int) ([]groupprofiles.Profile, error)
	UpdateGroupProfile(id string, req groupprofiles.UpdateProfileRequest) (groupprofiles.Profile, error)
	DeleteGroupProfile(id string) error
	AssignGroupProfile(groupID, profileID, assignedBy string) (groupprofiles.Assignment, error)
	GetGroupProfileAssignment(groupID string) (groupprofiles.Assignment, bool, error)
	RemoveGroupProfileAssignment(groupID string) error
	RecordDriftCheck(check groupprofiles.DriftCheck) (groupprofiles.DriftCheck, error)
	ListDriftChecks(groupID string, limit int) ([]groupprofiles.DriftCheck, error)
}

// FailoverStore provides persistence for group failover pair configuration.
type FailoverStore interface {
	CreateFailoverPair(req groupfailover.CreatePairRequest) (groupfailover.FailoverPair, error)
	GetFailoverPair(id string) (groupfailover.FailoverPair, bool, error)
	ListFailoverPairs(limit int) ([]groupfailover.FailoverPair, error)
	UpdateFailoverPair(id string, req groupfailover.UpdatePairRequest) (groupfailover.FailoverPair, error)
	DeleteFailoverPair(id string) error
	UpdateFailoverReadiness(id string, score int, checkedAt time.Time) error
}

// ReliabilityHistoryStore provides persistence for historical group reliability
// snapshots that are materialized from the live group health computation.
type ReliabilityHistoryStore interface {
	InsertReliabilityRecord(groupID string, score int, grade string, factors map[string]any, windowHours int) error
	ListReliabilityHistory(groupID string, days int) ([]ReliabilityRecord, error)
	PruneReliabilityHistory(olderThanDays int) (int64, error)
}

// SettingsStore manages system-level key-value settings.
type SettingsStore interface {
	GetSystemSetting(ctx context.Context, key string) (json.RawMessage, bool, error)
	PutSystemSetting(ctx context.Context, key string, value json.RawMessage) error
}

// FileConnection represents a saved remote file system connection (SFTP, FTP, SMB, WebDAV).
type FileConnection struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Protocol     string         `json:"protocol"`
	Host         string         `json:"host"`
	Port         *int           `json:"port,omitempty"`
	InitialPath  string         `json:"initial_path"`
	CredentialID *string        `json:"credential_id,omitempty"`
	ExtraConfig  map[string]any `json:"extra_config,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// RemoteBookmark is a saved remote desktop connection to an external host.
type RemoteBookmark struct {
	ID             string    `json:"id"`
	Label          string    `json:"label"`
	Protocol       string    `json:"protocol"`
	Host           string    `json:"host"`
	Port           int       `json:"port"`
	CredentialID   *string   `json:"credential_id,omitempty"`
	HasCredentials bool      `json:"has_credentials"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// FileTransfer represents a file transfer job between two endpoints.
type FileTransfer struct {
	ID               string     `json:"id"`
	SourceType       string     `json:"source_type"`
	SourceID         string     `json:"source_id"`
	SourcePath       string     `json:"source_path"`
	DestType         string     `json:"dest_type"`
	DestID           string     `json:"dest_id"`
	DestPath         string     `json:"dest_path"`
	FileName         string     `json:"file_name"`
	FileSize         *int64     `json:"file_size,omitempty"`
	BytesTransferred int64      `json:"bytes_transferred"`
	Status           string     `json:"status"`
	Error            *string    `json:"error,omitempty"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}

// FileConnectionStore provides persistence for remote file connection profiles.
type FileConnectionStore interface {
	ListFileConnections(ctx context.Context) ([]FileConnection, error)
	GetFileConnection(ctx context.Context, id string) (*FileConnection, error)
	CreateFileConnection(ctx context.Context, fc *FileConnection) error
	UpdateFileConnection(ctx context.Context, fc *FileConnection) error
	DeleteFileConnection(ctx context.Context, id string) error
}

// RemoteBookmarkStore provides persistence for saved remote desktop bookmarks.
type RemoteBookmarkStore interface {
	ListRemoteBookmarks(ctx context.Context) ([]RemoteBookmark, error)
	GetRemoteBookmark(ctx context.Context, id string) (*RemoteBookmark, error)
	CreateRemoteBookmark(ctx context.Context, bm *RemoteBookmark) error
	UpdateRemoteBookmark(ctx context.Context, bm RemoteBookmark) error
	DeleteRemoteBookmark(ctx context.Context, id string) error
}

// FileTransferStore provides persistence for file transfer jobs.
type FileTransferStore interface {
	GetFileTransfer(ctx context.Context, id string) (*FileTransfer, error)
	CreateFileTransfer(ctx context.Context, ft *FileTransfer) error
	UpdateFileTransfer(ctx context.Context, ft *FileTransfer) error
	ListActiveFileTransfers(ctx context.Context) ([]FileTransfer, error)
}

// ScheduleStore provides persistence for scheduled tasks.
type ScheduleStore interface {
	CreateScheduledTask(ctx context.Context, task schedules.ScheduledTask) error
	GetScheduledTask(ctx context.Context, id string) (schedules.ScheduledTask, bool, error)
	ListScheduledTasks(ctx context.Context) ([]schedules.ScheduledTask, error)
	UpdateScheduledTask(ctx context.Context, id string, name *string, cronExpr *string, command *string, targets *[]string, enabled *bool) error
	DeleteScheduledTask(ctx context.Context, id string) error
}

// WebhookStore provides persistence for webhook subscriptions.
type WebhookStore interface {
	CreateWebhook(ctx context.Context, wh webhooks.Webhook) error
	GetWebhook(ctx context.Context, id string) (webhooks.Webhook, bool, error)
	ListWebhooks(ctx context.Context) ([]webhooks.Webhook, error)
	UpdateWebhook(ctx context.Context, id string, name *string, url *string, events *[]string, enabled *bool) error
	DeleteWebhook(ctx context.Context, id string) error
}

// SavedActionStore provides persistence for reusable saved action sequences.
type SavedActionStore interface {
	CreateSavedAction(ctx context.Context, action savedactions.SavedAction) error
	GetSavedAction(ctx context.Context, id string) (savedactions.SavedAction, bool, error)
	ListSavedActions(ctx context.Context) ([]savedactions.SavedAction, error)
	DeleteSavedAction(ctx context.Context, id string) error
}
