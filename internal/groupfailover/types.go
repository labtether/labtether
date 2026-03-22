package groupfailover

import (
	"errors"
	"time"
)

var (
	ErrPairNotFound = errors.New("failover pair not found")
)

type FailoverPair struct {
	ID                   string         `json:"id"`
	PrimaryGroupID       string         `json:"primary_group_id"`
	BackupGroupID        string         `json:"backup_group_id"`
	Name                 string         `json:"name,omitempty"`
	RequiredCapabilities map[string]any `json:"required_capabilities,omitempty"`
	ReadinessScore       int            `json:"readiness_score"`
	LastCheckedAt        *time.Time     `json:"last_checked_at,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}

type CreatePairRequest struct {
	PrimaryGroupID       string         `json:"primary_group_id"`
	BackupGroupID        string         `json:"backup_group_id"`
	Name                 string         `json:"name,omitempty"`
	RequiredCapabilities map[string]any `json:"required_capabilities,omitempty"`
}

type UpdatePairRequest struct {
	Name                 *string        `json:"name,omitempty"`
	PrimaryGroupID       *string        `json:"primary_group_id,omitempty"`
	BackupGroupID        *string        `json:"backup_group_id,omitempty"`
	RequiredCapabilities map[string]any `json:"required_capabilities,omitempty"`
}
