package groupprofiles

import (
	"errors"
	"strings"
	"time"
)

const (
	DriftStatusCompliant = "compliant"
	DriftStatusDrifted   = "drifted"
)

var (
	ErrProfileNotFound = errors.New("group profile not found")
)

type Profile struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Config      map[string]any `json:"config"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type CreateProfileRequest struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Config      map[string]any `json:"config"`
}

type UpdateProfileRequest struct {
	Name        *string         `json:"name,omitempty"`
	Description *string         `json:"description,omitempty"`
	Config      *map[string]any `json:"config,omitempty"`
}

type Assignment struct {
	ID         string    `json:"id"`
	GroupID    string    `json:"group_id"`
	ProfileID  string    `json:"profile_id"`
	AssignedBy string    `json:"assigned_by,omitempty"`
	AssignedAt time.Time `json:"assigned_at"`
}

type DriftCheck struct {
	ID           string         `json:"id"`
	GroupID      string         `json:"group_id"`
	ProfileID    string         `json:"profile_id"`
	Status       string         `json:"status"`
	DriftDetails map[string]any `json:"drift_details,omitempty"`
	CheckedAt    time.Time      `json:"checked_at"`
}

func NormalizeDriftStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case DriftStatusCompliant:
		return DriftStatusCompliant
	case DriftStatusDrifted:
		return DriftStatusDrifted
	default:
		return ""
	}
}
