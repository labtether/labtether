package model

import "time"

type RelationshipType string

const (
	RelationshipContains     RelationshipType = "contains"
	RelationshipRunsOn       RelationshipType = "runs_on"
	RelationshipDependsOn    RelationshipType = "depends_on"
	RelationshipConnectedTo  RelationshipType = "connected_to"
	RelationshipBacksUp      RelationshipType = "backs_up"
	RelationshipReplicatesTo RelationshipType = "replicates_to"
	RelationshipManagedBy    RelationshipType = "managed_by"
	RelationshipMemberOf     RelationshipType = "member_of"
)

type RelationshipDirection string

const (
	RelationshipDirectionUpstream      RelationshipDirection = "upstream"
	RelationshipDirectionDownstream    RelationshipDirection = "downstream"
	RelationshipDirectionBidirectional RelationshipDirection = "bidirectional"
)

type RelationshipCriticality string

const (
	RelationshipCriticalityCritical RelationshipCriticality = "critical"
	RelationshipCriticalityHigh     RelationshipCriticality = "high"
	RelationshipCriticalityMedium   RelationshipCriticality = "medium"
	RelationshipCriticalityLow      RelationshipCriticality = "low"
)

type ResourceRelationship struct {
	ID               string                  `json:"id"`
	SourceResourceID string                  `json:"source_resource_id"`
	TargetResourceID string                  `json:"target_resource_id"`
	Type             RelationshipType        `json:"type"`
	Direction        RelationshipDirection   `json:"direction"`
	Criticality      RelationshipCriticality `json:"criticality"`
	Inferred         bool                    `json:"inferred"`
	Confidence       int                     `json:"confidence"`
	Evidence         map[string]any          `json:"evidence,omitempty"`
	CreatedAt        time.Time               `json:"created_at"`
	UpdatedAt        time.Time               `json:"updated_at"`
}
