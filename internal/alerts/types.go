package alerts

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityMedium   = "medium"
	SeverityLow      = "low"

	RuleStatusActive = "active"
	RuleStatusPaused = "paused"

	RuleKindMetricThreshold = "metric_threshold"
	RuleKindMetricDeadman   = "metric_deadman"
	RuleKindHeartbeatStale  = "heartbeat_stale"
	RuleKindLogPattern      = "log_pattern"
	RuleKindComposite       = "composite"
	RuleKindSyntheticCheck  = "synthetic_check"

	TargetScopeAsset  = "asset"
	TargetScopeGroup  = "group"
	TargetScopeGlobal = "global"

	EvaluationStatusOK         = "ok"
	EvaluationStatusTriggered  = "triggered"
	EvaluationStatusSuppressed = "suppressed"
	EvaluationStatusError      = "error"

	InstanceStatusPending      = "pending"
	InstanceStatusFiring       = "firing"
	InstanceStatusAcknowledged = "acknowledged"
	InstanceStatusResolved     = "resolved"
)

var (
	ErrRuleNotFound         = errors.New("alert rule not found")
	ErrRuleValidation       = errors.New("invalid alert rule")
	ErrRuleTargetValidation = errors.New("invalid alert rule target")
)

type RuleTargetInput struct {
	AssetID  string         `json:"asset_id,omitempty"`
	GroupID  string         `json:"group_id,omitempty"`
	Selector map[string]any `json:"selector,omitempty"`
}

type RuleTarget struct {
	ID        string         `json:"id"`
	RuleID    string         `json:"rule_id"`
	AssetID   string         `json:"asset_id,omitempty"`
	GroupID   string         `json:"group_id,omitempty"`
	Selector  map[string]any `json:"selector,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

type CreateRuleRequest struct {
	Name                      string            `json:"name"`
	Description               string            `json:"description,omitempty"`
	Status                    string            `json:"status,omitempty"`
	Kind                      string            `json:"kind"`
	Severity                  string            `json:"severity"`
	TargetScope               string            `json:"target_scope"`
	CooldownSeconds           int               `json:"cooldown_seconds,omitempty"`
	ReopenAfterSeconds        int               `json:"reopen_after_seconds,omitempty"`
	EvaluationIntervalSeconds int               `json:"evaluation_interval_seconds,omitempty"`
	WindowSeconds             int               `json:"window_seconds,omitempty"`
	Condition                 map[string]any    `json:"condition"`
	Labels                    map[string]string `json:"labels,omitempty"`
	Metadata                  map[string]string `json:"metadata,omitempty"`
	Targets                   []RuleTargetInput `json:"targets,omitempty"`
	CreatedBy                 string            `json:"created_by,omitempty"`
}

type UpdateRuleRequest struct {
	Name                      *string            `json:"name,omitempty"`
	Description               *string            `json:"description,omitempty"`
	Status                    *string            `json:"status,omitempty"`
	Severity                  *string            `json:"severity,omitempty"`
	CooldownSeconds           *int               `json:"cooldown_seconds,omitempty"`
	ReopenAfterSeconds        *int               `json:"reopen_after_seconds,omitempty"`
	EvaluationIntervalSeconds *int               `json:"evaluation_interval_seconds,omitempty"`
	WindowSeconds             *int               `json:"window_seconds,omitempty"`
	Condition                 *map[string]any    `json:"condition,omitempty"`
	Labels                    *map[string]string `json:"labels,omitempty"`
	Metadata                  *map[string]string `json:"metadata,omitempty"`
	Targets                   *[]RuleTargetInput `json:"targets,omitempty"`
}

type Rule struct {
	ID                        string            `json:"id"`
	Name                      string            `json:"name"`
	Description               string            `json:"description,omitempty"`
	Status                    string            `json:"status"`
	Kind                      string            `json:"kind"`
	Severity                  string            `json:"severity"`
	TargetScope               string            `json:"target_scope"`
	CooldownSeconds           int               `json:"cooldown_seconds"`
	ReopenAfterSeconds        int               `json:"reopen_after_seconds"`
	EvaluationIntervalSeconds int               `json:"evaluation_interval_seconds"`
	WindowSeconds             int               `json:"window_seconds"`
	Condition                 map[string]any    `json:"condition"`
	Labels                    map[string]string `json:"labels,omitempty"`
	Metadata                  map[string]string `json:"metadata,omitempty"`
	Targets                   []RuleTarget      `json:"targets,omitempty"`
	CreatedBy                 string            `json:"created_by"`
	CreatedAt                 time.Time         `json:"created_at"`
	UpdatedAt                 time.Time         `json:"updated_at"`
	LastEvaluatedAt           *time.Time        `json:"last_evaluated_at,omitempty"`
}

type Evaluation struct {
	ID             string         `json:"id"`
	RuleID         string         `json:"rule_id"`
	Status         string         `json:"status"`
	EvaluatedAt    time.Time      `json:"evaluated_at"`
	DurationMS     int            `json:"duration_ms"`
	CandidateCount int            `json:"candidate_count"`
	TriggeredCount int            `json:"triggered_count"`
	Error          string         `json:"error,omitempty"`
	Details        map[string]any `json:"details,omitempty"`
}

func NormalizeSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case SeverityCritical:
		return SeverityCritical
	case SeverityHigh:
		return SeverityHigh
	case SeverityMedium:
		return SeverityMedium
	case SeverityLow:
		return SeverityLow
	default:
		return ""
	}
}

func NormalizeRuleStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case RuleStatusActive:
		return RuleStatusActive
	case RuleStatusPaused:
		return RuleStatusPaused
	default:
		return ""
	}
}

func NormalizeRuleKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case RuleKindMetricThreshold:
		return RuleKindMetricThreshold
	case RuleKindMetricDeadman:
		return RuleKindMetricDeadman
	case RuleKindHeartbeatStale:
		return RuleKindHeartbeatStale
	case RuleKindLogPattern:
		return RuleKindLogPattern
	case RuleKindComposite:
		return RuleKindComposite
	case RuleKindSyntheticCheck:
		return RuleKindSyntheticCheck
	default:
		return ""
	}
}

func NormalizeTargetScope(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case TargetScopeAsset:
		return TargetScopeAsset
	case TargetScopeGroup:
		return TargetScopeGroup
	case TargetScopeGlobal:
		return TargetScopeGlobal
	default:
		return ""
	}
}

func NormalizeEvaluationStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case EvaluationStatusOK:
		return EvaluationStatusOK
	case EvaluationStatusTriggered:
		return EvaluationStatusTriggered
	case EvaluationStatusSuppressed:
		return EvaluationStatusSuppressed
	case EvaluationStatusError:
		return EvaluationStatusError
	default:
		return ""
	}
}

func TargetReferenceCount(target RuleTargetInput) int {
	count := 0
	if strings.TrimSpace(target.AssetID) != "" {
		count++
	}
	if strings.TrimSpace(target.GroupID) != "" {
		count++
	}
	if len(target.Selector) > 0 {
		count++
	}
	return count
}

// AlertInstance represents a fired alert instance.
type AlertInstance struct {
	ID           string            `json:"id"`
	RuleID       string            `json:"rule_id"`
	Fingerprint  string            `json:"fingerprint"`
	Status       string            `json:"status"`
	Severity     string            `json:"severity"`
	Labels       map[string]string `json:"labels,omitempty"`
	Annotations  map[string]string `json:"annotations,omitempty"`
	StartedAt    time.Time         `json:"started_at"`
	ResolvedAt   *time.Time        `json:"resolved_at,omitempty"`
	LastFiredAt  time.Time         `json:"last_fired_at"`
	SuppressedBy string            `json:"suppressed_by,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// CreateInstanceRequest is used to create a new alert instance.
type CreateInstanceRequest struct {
	RuleID      string            `json:"rule_id"`
	Fingerprint string            `json:"fingerprint"`
	Severity    string            `json:"severity"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// AlertSilence suppresses alerts matching its matchers.
type AlertSilence struct {
	ID        string            `json:"id"`
	Matchers  map[string]string `json:"matchers"`
	Reason    string            `json:"reason,omitempty"`
	CreatedBy string            `json:"created_by"`
	StartsAt  time.Time         `json:"starts_at"`
	EndsAt    time.Time         `json:"ends_at"`
	CreatedAt time.Time         `json:"created_at"`
}

// CreateSilenceRequest is used to create an alert silence.
type CreateSilenceRequest struct {
	Matchers  map[string]string `json:"matchers"`
	Reason    string            `json:"reason,omitempty"`
	CreatedBy string            `json:"created_by,omitempty"`
	StartsAt  time.Time         `json:"starts_at"`
	EndsAt    time.Time         `json:"ends_at"`
}

func NormalizeInstanceStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case InstanceStatusPending:
		return InstanceStatusPending
	case InstanceStatusFiring:
		return InstanceStatusFiring
	case InstanceStatusAcknowledged:
		return InstanceStatusAcknowledged
	case InstanceStatusResolved:
		return InstanceStatusResolved
	default:
		return ""
	}
}

func CanTransitionInstanceStatus(from, to string) bool {
	from = NormalizeInstanceStatus(from)
	to = NormalizeInstanceStatus(to)
	if from == "" || to == "" {
		return false
	}
	if from == to {
		return true
	}
	switch from {
	case InstanceStatusPending:
		return to == InstanceStatusFiring || to == InstanceStatusResolved
	case InstanceStatusFiring:
		return to == InstanceStatusAcknowledged || to == InstanceStatusResolved
	case InstanceStatusAcknowledged:
		return to == InstanceStatusFiring || to == InstanceStatusResolved
	case InstanceStatusResolved:
		return false
	default:
		return false
	}
}

// GenerateFingerprint produces a stable fingerprint from rule ID and labels.
func GenerateFingerprint(ruleID string, labels map[string]string) string {
	h := sha256.New()
	h.Write([]byte(ruleID))
	h.Write([]byte{0})
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(labels[k]))
		h.Write([]byte{0})
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
