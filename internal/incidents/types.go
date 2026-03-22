package incidents

import (
	"errors"
	"strings"
	"time"
)

const (
	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityMedium   = "medium"
	SeverityLow      = "low"

	StatusOpen          = "open"
	StatusInvestigating = "investigating"
	StatusMitigated     = "mitigated"
	StatusResolved      = "resolved"
	StatusClosed        = "closed"

	SourceManual    = "manual"
	SourceAlertAuto = "alert_auto"

	LinkTypeTrigger = "trigger"
	LinkTypeRelated = "related"
	LinkTypeSymptom = "symptom"
	LinkTypeCause   = "cause"
)

var (
	ErrIncidentNotFound          = errors.New("incident not found")
	ErrInvalidStatusTransition   = errors.New("invalid incident status transition")
	ErrAlertReferenceRequired    = errors.New("incident alert link reference is required")
	ErrIncidentAlertLinkConflict = errors.New("incident alert link already exists")
	ErrIncidentAssetConflict     = errors.New("asset already linked to incident")
)

type CreateIncidentRequest struct {
	Title          string            `json:"title"`
	Summary        string            `json:"summary,omitempty"`
	Severity       string            `json:"severity"`
	Source         string            `json:"source,omitempty"`
	GroupID        string            `json:"group_id,omitempty"`
	PrimaryAssetID string            `json:"primary_asset_id,omitempty"`
	Assignee       string            `json:"assignee,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	CreatedBy      string            `json:"created_by,omitempty"`
}

type UpdateIncidentRequest struct {
	Title          *string            `json:"title,omitempty"`
	Summary        *string            `json:"summary,omitempty"`
	Status         *string            `json:"status,omitempty"`
	Severity       *string            `json:"severity,omitempty"`
	GroupID        *string            `json:"group_id,omitempty"`
	PrimaryAssetID *string            `json:"primary_asset_id,omitempty"`
	Assignee       *string            `json:"assignee,omitempty"`
	Metadata       *map[string]string `json:"metadata,omitempty"`
	RootCause      *string            `json:"root_cause,omitempty"`
	ActionItems    *[]string          `json:"action_items,omitempty"`
	LessonsLearned *string            `json:"lessons_learned,omitempty"`
}

type Incident struct {
	ID             string            `json:"id"`
	Title          string            `json:"title"`
	Summary        string            `json:"summary,omitempty"`
	Status         string            `json:"status"`
	Severity       string            `json:"severity"`
	Source         string            `json:"source"`
	GroupID        string            `json:"group_id,omitempty"`
	PrimaryAssetID string            `json:"primary_asset_id,omitempty"`
	Assignee       string            `json:"assignee,omitempty"`
	CreatedBy      string            `json:"created_by"`
	OpenedAt       time.Time         `json:"opened_at"`
	MitigatedAt    *time.Time        `json:"mitigated_at,omitempty"`
	ResolvedAt     *time.Time        `json:"resolved_at,omitempty"`
	ClosedAt       *time.Time        `json:"closed_at,omitempty"`
	RootCause      string            `json:"root_cause,omitempty"`
	ActionItems    []string          `json:"action_items,omitempty"`
	LessonsLearned string            `json:"lessons_learned,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

type LinkAlertRequest struct {
	AlertRuleID      string `json:"alert_rule_id,omitempty"`
	AlertInstanceID  string `json:"alert_instance_id,omitempty"`
	AlertFingerprint string `json:"alert_fingerprint,omitempty"`
	LinkType         string `json:"link_type"`
	CreatedBy        string `json:"created_by,omitempty"`
}

type AlertLink struct {
	ID               string    `json:"id"`
	IncidentID       string    `json:"incident_id"`
	AlertRuleID      string    `json:"alert_rule_id,omitempty"`
	AlertInstanceID  string    `json:"alert_instance_id,omitempty"`
	AlertFingerprint string    `json:"alert_fingerprint,omitempty"`
	LinkType         string    `json:"link_type"`
	CreatedBy        string    `json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
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

func NormalizeStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case StatusOpen:
		return StatusOpen
	case StatusInvestigating:
		return StatusInvestigating
	case StatusMitigated:
		return StatusMitigated
	case StatusResolved:
		return StatusResolved
	case StatusClosed:
		return StatusClosed
	default:
		return ""
	}
}

func NormalizeSource(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case SourceManual:
		return SourceManual
	case SourceAlertAuto:
		return SourceAlertAuto
	default:
		return ""
	}
}

func NormalizeLinkType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case LinkTypeTrigger:
		return LinkTypeTrigger
	case LinkTypeRelated:
		return LinkTypeRelated
	case LinkTypeSymptom:
		return LinkTypeSymptom
	case LinkTypeCause:
		return LinkTypeCause
	default:
		return ""
	}
}

const (
	AssetRolePrimary      = "primary"
	AssetRoleImpacted     = "impacted"
	AssetRoleRelated      = "related"
	AssetRoleContributing = "contributing"
)

// LinkAssetRequest is the payload to link an asset to an incident.
type LinkAssetRequest struct {
	AssetID string `json:"asset_id"`
	Role    string `json:"role"`
}

// IncidentAsset represents a linked asset on an incident.
type IncidentAsset struct {
	ID         string    `json:"id"`
	IncidentID string    `json:"incident_id"`
	AssetID    string    `json:"asset_id"`
	Role       string    `json:"role"`
	CreatedAt  time.Time `json:"created_at"`
}

func NormalizeAssetRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case AssetRolePrimary:
		return AssetRolePrimary
	case AssetRoleImpacted:
		return AssetRoleImpacted
	case AssetRoleRelated:
		return AssetRoleRelated
	case AssetRoleContributing:
		return AssetRoleContributing
	default:
		return ""
	}
}

func CanTransitionStatus(from, to string) bool {
	from = NormalizeStatus(from)
	to = NormalizeStatus(to)
	if from == "" || to == "" {
		return false
	}
	if from == to {
		return true
	}

	switch from {
	case StatusOpen:
		return to == StatusInvestigating || to == StatusResolved || to == StatusClosed
	case StatusInvestigating:
		return to == StatusMitigated || to == StatusResolved || to == StatusClosed
	case StatusMitigated:
		return to == StatusInvestigating || to == StatusResolved || to == StatusClosed
	case StatusResolved:
		return to == StatusClosed || to == StatusInvestigating
	case StatusClosed:
		return to == StatusInvestigating
	default:
		return false
	}
}
