package persistence

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/retention"
	"github.com/labtether/labtether/internal/savedactions"
	"github.com/labtether/labtether/internal/schedules"
	"github.com/labtether/labtether/internal/synthetic"
	"github.com/labtether/labtether/internal/updates"
	"github.com/labtether/labtether/internal/webhooks"
	"time"
)

// ErrNotFound is returned when a requested resource does not exist.
var ErrNotFound = errors.New("not found")

// ErrAlreadyExists is returned when a requested create operation conflicts with
// an existing record.
var ErrAlreadyExists = errors.New("already exists")

// ErrUpdatePlanActive is returned when an update plan still has queued or
// running work. Callers must leave both the plan and its runs intact and retry
// deletion only after every associated run reaches a terminal state.
var ErrUpdatePlanActive = errors.New("update plan has queued or running runs")

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

// UpdateRunPageStore is an optional optimization interface for paginating
// update runs without being constrained by the primary ListUpdateRuns limit.
type UpdateRunPageStore interface {
	ListUpdateRunsPage(limit, offset int, status string) ([]updates.Run, error)
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
	UpdateCredentialProfile(profile credentials.Profile) (credentials.Profile, error)
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

const (
	CredentialProfilePerOwnerLimit = 100
	CredentialProfileGlobalLimit   = 500
)

var (
	ErrCredentialProfileOwnerLimit  = errors.New("credential profile owner limit reached")
	ErrCredentialProfileGlobalLimit = errors.New("credential profile global limit reached")
	ErrCredentialProfileInUse       = errors.New("credential profile is in use")
	ErrCredentialProfileProtected   = errors.New("credential profile is protected")
)

// CredentialProfileReference is a redacted aggregate reference count. It
// intentionally excludes asset names, hosts, usernames, and configuration.
type CredentialProfileReference struct {
	Resource string `json:"resource"`
	Count    int    `json:"count"`
}

// CredentialProfileReferenceSummary describes all live bindings that prevent
// safe deletion of one credential profile.
type CredentialProfileReferenceSummary struct {
	Total      int                          `json:"total"`
	References []CredentialProfileReference `json:"references"`
}

// CredentialProfileLifecycleStore is the atomic lifecycle extension used by
// the global credential manager. Keeping it separate preserves the narrower
// runtime lookup contract used by protocol subsystems and test doubles.
type CredentialProfileLifecycleStore interface {
	CreateCredentialProfileBounded(profile credentials.Profile, ownerID string, perOwnerLimit, globalLimit int) (credentials.Profile, error)
	DeleteCredentialProfileIfUnreferenced(id string) (CredentialProfileReferenceSummary, error)
}

// AuthStore provides persistence for user accounts and sessions.
type AuthStore interface {
	GetUserByID(id string) (auth.User, bool, error)
	GetUserByUsername(username string) (auth.User, bool, error)
	GetUserByOIDCIdentity(provider, issuer, subject string) (auth.User, bool, error)
	// GetUserByOIDCSubject is retained only for issuer-less users created before
	// issuer-scoped OIDC identity bindings were introduced.
	GetUserByOIDCSubject(provider, subject string) (auth.User, bool, error)
	ListUsers(limit int) ([]auth.User, error)
	BootstrapFirstUser(username, passwordHash string) (auth.User, bool, error)
	CreateUser(username, passwordHash string) (auth.User, error)
	CreateUserWithRole(username, passwordHash, role, authProvider, oidcSubject string) (auth.User, error)
	CreateUserWithOIDCIdentity(username, passwordHash, role, authProvider, oidcIssuer, oidcSubject string) (auth.User, error)
	// BindLegacyOIDCIdentity atomically assigns an issuer to one matching
	// issuer-less OIDC user. The boolean is false when the legacy binding no
	// longer matches, allowing callers to resolve concurrent adoption safely.
	BindLegacyOIDCIdentity(id, authProvider, oidcSubject, oidcIssuer string) (auth.User, bool, error)
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
	UpdateAPIKey(ctx context.Context, id string, name *string, scopes *[]string, allowedAssets *[]string, expiresAt **time.Time) error
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
	ListNotificationChannels(limit, offset int) ([]notifications.Channel, error)
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
	UpdateRetryState(ctx context.Context, id string, retryCount int, nextRetryAt *time.Time, status, errorMessage string, payload map[string]any) error
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

// DueSyntheticCheckStore is an optional optimization interface for loading
// enabled synthetic checks that are currently due to run, ordered by oldest
// last-run first so older checks do not starve behind frequently updated rows.
type DueSyntheticCheckStore interface {
	ListDueSyntheticChecks(ctx context.Context, now time.Time, limit int) ([]synthetic.Check, error)
}

// SyntheticMetricSnapshot is the latest persisted result for one enabled
// synthetic check. Target URLs are deliberately excluded because they may
// contain credentials or high-cardinality query values and must never become
// telemetry labels.
type SyntheticMetricSnapshot struct {
	CheckID   string
	CheckName string
	CheckType string
	ResultID  string
	Status    string
	LatencyMS *int
	CheckedAt time.Time
}

// SyntheticMetricSnapshotStore loads latest check results with one bounded,
// deadline-aware read instead of one ListSyntheticResults query per check.
type SyntheticMetricSnapshotStore interface {
	LatestSyntheticMetricSnapshots(ctx context.Context, maxChecks int) ([]SyntheticMetricSnapshot, error)
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
type CreateEnrollmentTokenParams struct {
	TokenHash      string
	Label          string
	ExpiresAt      time.Time
	MaxUses        int
	Scope          string
	AssetID        string
	AllowedGroupID string
	CreatedBy      string
}

type EnrollmentStore interface {
	CreateEnrollmentToken(params CreateEnrollmentTokenParams) (enrollment.EnrollmentToken, error)
	ValidateEnrollmentToken(tokenHash string) (enrollment.EnrollmentToken, bool, error)
	ConsumeEnrollmentToken(tokenHash string) (enrollment.EnrollmentToken, bool, error)
	IncrementEnrollmentTokenUse(id string) error
	RevokeEnrollmentToken(id string) error
	ListEnrollmentTokens(limit int) ([]enrollment.EnrollmentToken, error)
	CreateAgentToken(assetID, tokenHash, enrolledVia string, expiresAt time.Time) (enrollment.AgentToken, error)
	RotateAgentToken(assetID, tokenHash, enrolledVia string, expiresAt time.Time) (enrollment.AgentToken, error)
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
	GetLinkSuggestion(id string) (LinkSuggestion, bool, error)
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

// ScheduleStore provides persistence for scheduled tasks.
type ScheduleStore interface {
	CreateScheduledTask(ctx context.Context, task schedules.ScheduledTask) error
	GetScheduledTask(ctx context.Context, id string) (schedules.ScheduledTask, bool, error)
	ListScheduledTasks(ctx context.Context, limit, offset int) ([]schedules.ScheduledTask, int, error)
	UpdateScheduledTask(ctx context.Context, id string, name *string, cronExpr *string, command *string, targets *[]string, groupID *string, enabled *bool, nextRun schedules.NextRunUpdate) error
	DeleteScheduledTask(ctx context.Context, id string) error
}

// ScheduleExecutionStore is the production scheduler's durable outbox. A due
// schedule advance and its leased job-queue insertion must commit atomically so
// multiple hub instances cannot dispatch the same occurrence twice.
type ScheduleExecutionStore interface {
	// ListScheduledTasksForEvaluation returns at most limit enabled definitions
	// that need work now, ordered so repeated bounded passes drain the backlog.
	ListScheduledTasksForEvaluation(ctx context.Context, now time.Time, limit int) ([]schedules.ScheduledTask, error)
	InitializeScheduledTaskNextRun(ctx context.Context, id string, nextRunAt time.Time) (bool, error)
	ClaimScheduledTaskExecution(ctx context.Context, claim schedules.ExecutionClaim) (bool, error)
	BeginScheduledTaskExecution(ctx context.Context, scheduleID, jobID string) (bool, error)
	MarkScheduledTaskInvalid(ctx context.Context, id, errorMessage string) error
	CompleteScheduledTaskExecution(ctx context.Context, scheduleID, jobID, status, errorMessage string, completedAt time.Time) error
}

// WebhookStore provides persistence for webhook subscriptions.
type WebhookStore interface {
	CreateWebhook(ctx context.Context, wh webhooks.Webhook) error
	GetWebhook(ctx context.Context, id string) (webhooks.Webhook, bool, error)
	ListWebhooks(ctx context.Context) ([]webhooks.Webhook, error)
	UpdateWebhook(ctx context.Context, wh webhooks.Webhook) error
	DeleteWebhook(ctx context.Context, id string) error
	MarkWebhookTriggered(ctx context.Context, id string, at time.Time) error
}

// SavedActionStore provides persistence for reusable saved action sequences.
type SavedActionStore interface {
	CreateSavedAction(ctx context.Context, action savedactions.SavedAction) error
	GetSavedAction(ctx context.Context, actorID, id string) (savedactions.SavedAction, bool, error)
	ListSavedActions(ctx context.Context, actorID string, limit, offset int) ([]savedactions.SavedAction, int, error)
	DeleteSavedAction(ctx context.Context, actorID, id string) error
}
