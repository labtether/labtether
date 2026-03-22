package groupmaintenance

import (
	"time"
)

const (
	OverrideTypeAction = "action"
	OverrideTypeUpdate = "update"
)

type MaintenanceOverride struct {
	ID                  string    `json:"id"`
	MaintenanceWindowID string    `json:"maintenance_window_id"`
	OverrideType        string    `json:"override_type"`
	Reason              string    `json:"reason"`
	ReferenceID         string    `json:"reference_id,omitempty"`
	ApprovedBy          string    `json:"approved_by,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}

type CreateOverrideRequest struct {
	MaintenanceWindowID string `json:"maintenance_window_id"`
	OverrideType        string `json:"override_type"`
	Reason              string `json:"reason"`
	ReferenceID         string `json:"reference_id,omitempty"`
	ApprovedBy          string `json:"approved_by,omitempty"`
}
