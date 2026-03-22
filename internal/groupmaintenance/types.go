package groupmaintenance

import (
	"errors"
	"time"
)

var (
	// ErrGroupNotFound indicates a referenced group does not exist.
	ErrGroupNotFound = errors.New("group not found")
	// ErrMaintenanceWindowNotFound indicates a maintenance window does not exist.
	ErrMaintenanceWindowNotFound = errors.New("maintenance window not found")
)

// MaintenanceWindow defines a group-scoped change-control period.
type MaintenanceWindow struct {
	ID             string    `json:"id"`
	GroupID        string    `json:"group_id"`
	Name           string    `json:"name"`
	StartAt        time.Time `json:"start_at"`
	EndAt          time.Time `json:"end_at"`
	SuppressAlerts bool      `json:"suppress_alerts"`
	BlockActions   bool      `json:"block_actions"`
	BlockUpdates   bool      `json:"block_updates"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// CreateMaintenanceWindowRequest contains fields for creating a maintenance window.
type CreateMaintenanceWindowRequest struct {
	Name           string    `json:"name"`
	StartAt        time.Time `json:"start_at"`
	EndAt          time.Time `json:"end_at"`
	SuppressAlerts bool      `json:"suppress_alerts"`
	BlockActions   bool      `json:"block_actions"`
	BlockUpdates   bool      `json:"block_updates"`
}

// UpdateMaintenanceWindowRequest contains mutable maintenance window fields.
type UpdateMaintenanceWindowRequest struct {
	Name           string    `json:"name"`
	StartAt        time.Time `json:"start_at"`
	EndAt          time.Time `json:"end_at"`
	SuppressAlerts bool      `json:"suppress_alerts"`
	BlockActions   bool      `json:"block_actions"`
	BlockUpdates   bool      `json:"block_updates"`
}
