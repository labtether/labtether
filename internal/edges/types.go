package edges

import (
	"strings"
	"time"
)

// Relationship types (same as dependencies package for backward compat)
const (
	RelContains    = "contains"
	RelRunsOn      = "runs_on"
	RelHostedOn    = "hosted_on"
	RelDependsOn   = "depends_on"
	RelProvidesTo  = "provides_to"
	RelConnectedTo = "connected_to"
)

// Origin values
const (
	OriginAuto      = "auto"
	OriginManual    = "manual"
	OriginSuggested = "suggested"
	OriginDismissed = "dismissed"
)

// Direction values (preserved from dependencies)
const (
	DirUpstream      = "upstream"
	DirDownstream    = "downstream"
	DirBidirectional = "bidirectional"
)

// Criticality values (preserved from dependencies)
const (
	CritCritical = "critical"
	CritHigh     = "high"
	CritMedium   = "medium"
	CritLow      = "low"
)

// ContainmentRelTypes are the relationship types that form the containment tree.
var ContainmentRelTypes = []string{RelContains, RelRunsOn, RelHostedOn}

// Edge represents a typed relationship between two assets.
type Edge struct {
	ID               string            `json:"id"`
	SourceAssetID    string            `json:"source_asset_id"`
	TargetAssetID    string            `json:"target_asset_id"`
	RelationshipType string            `json:"relationship_type"`
	Direction        string            `json:"direction"`
	Criticality      string            `json:"criticality"`
	Origin           string            `json:"origin"`
	Confidence       float64           `json:"confidence"`
	MatchSignals     map[string]any    `json:"match_signals,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// CreateEdgeRequest is the input for creating a new edge.
type CreateEdgeRequest struct {
	SourceAssetID    string            `json:"source_asset_id"`
	TargetAssetID    string            `json:"target_asset_id"`
	RelationshipType string            `json:"relationship_type"`
	Direction        string            `json:"direction"`
	Criticality      string            `json:"criticality"`
	Origin           string            `json:"origin"`
	Confidence       float64           `json:"confidence"`
	MatchSignals     map[string]any    `json:"match_signals,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

// TreeNode is an asset with its depth in the containment tree.
type TreeNode struct {
	AssetID string `json:"asset_id"`
	Depth   int    `json:"depth"`
}

// Composite represents a virtual identity binding.
type Composite struct {
	CompositeID string            `json:"composite_id"`
	Members     []CompositeMember `json:"members"`
}

// CompositeMember is one asset bound to a composite.
type CompositeMember struct {
	AssetID   string    `json:"asset_id"`
	Role      string    `json:"role"` // "primary" | "facet"
	CreatedAt time.Time `json:"created_at"`
}

// CreateCompositeRequest is the input for merging assets.
type CreateCompositeRequest struct {
	PrimaryAssetID string   `json:"primary_asset_id"`
	FacetAssetIDs  []string `json:"facet_asset_ids"`
}

// NormalizeOrigin normalizes an origin string to a valid constant.
func NormalizeOrigin(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case OriginAuto:
		return OriginAuto
	case OriginSuggested:
		return OriginSuggested
	case OriginDismissed:
		return OriginDismissed
	default:
		return OriginManual
	}
}

// AggregateConfidence combines multiple independent confidence scores.
// Formula: combined = 1 - ∏(1 - score_i)
func AggregateConfidence(scores []float64) float64 {
	if len(scores) == 0 {
		return 0.0
	}
	product := 1.0
	for _, s := range scores {
		product *= (1.0 - s)
	}
	return 1.0 - product
}
