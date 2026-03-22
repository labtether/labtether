package dependencies

import (
	"errors"
	"strings"
	"time"
)

const (
	RelationshipRunsOn      = "runs_on"
	RelationshipHostedOn    = "hosted_on"
	RelationshipDependsOn   = "depends_on"
	RelationshipProvidesTo  = "provides_to"
	RelationshipConnectedTo = "connected_to"
	RelationshipContains    = "contains"

	DirectionUpstream      = "upstream"
	DirectionDownstream    = "downstream"
	DirectionBidirectional = "bidirectional"

	CriticalityCritical = "critical"
	CriticalityHigh     = "high"
	CriticalityMedium   = "medium"
	CriticalityLow      = "low"
)

var (
	ErrDependencyNotFound  = errors.New("dependency not found")
	ErrSelfReference       = errors.New("source and target asset cannot be the same")
	ErrDuplicateDependency = errors.New("dependency already exists")
)

// Dependency represents a directed relationship between two assets.
type Dependency struct {
	ID               string            `json:"id"`
	SourceAssetID    string            `json:"source_asset_id"`
	TargetAssetID    string            `json:"target_asset_id"`
	RelationshipType string            `json:"relationship_type"`
	Direction        string            `json:"direction"`
	Criticality      string            `json:"criticality"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// CreateDependencyRequest is the payload to create a new dependency edge.
type CreateDependencyRequest struct {
	SourceAssetID    string            `json:"source_asset_id"`
	TargetAssetID    string            `json:"target_asset_id"`
	RelationshipType string            `json:"relationship_type"`
	Direction        string            `json:"direction"`
	Criticality      string            `json:"criticality"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

// ImpactNode represents one node in a blast-radius or upstream-causes traversal.
type ImpactNode struct {
	AssetID          string `json:"asset_id"`
	AssetName        string `json:"asset_name,omitempty"`
	Depth            int    `json:"depth"`
	RelationshipType string `json:"relationship_type"`
	Criticality      string `json:"criticality"`
}

func NormalizeRelationshipType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case RelationshipRunsOn:
		return RelationshipRunsOn
	case RelationshipHostedOn:
		return RelationshipHostedOn
	case RelationshipDependsOn:
		return RelationshipDependsOn
	case RelationshipProvidesTo:
		return RelationshipProvidesTo
	case RelationshipConnectedTo:
		return RelationshipConnectedTo
	case RelationshipContains:
		return RelationshipContains
	default:
		return ""
	}
}

func NormalizeDirection(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case DirectionUpstream:
		return DirectionUpstream
	case DirectionDownstream:
		return DirectionDownstream
	case DirectionBidirectional:
		return DirectionBidirectional
	default:
		return ""
	}
}

func NormalizeCriticality(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case CriticalityCritical:
		return CriticalityCritical
	case CriticalityHigh:
		return CriticalityHigh
	case CriticalityMedium:
		return CriticalityMedium
	case CriticalityLow:
		return CriticalityLow
	default:
		return ""
	}
}
