package groups

import (
	"encoding/json"
	"errors"
	"time"
)

// ErrGroupNotFound is returned when a referenced group does not exist.
var ErrGroupNotFound = errors.New("group not found")

// Group represents a hierarchical organizational unit for assets.
// Groups replace flat groupmaintenance with a tree structure supporting arbitrary nesting.
type Group struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Slug          string            `json:"slug"`
	ParentGroupID string            `json:"parent_group_id,omitempty"`
	Icon          string            `json:"icon,omitempty"`
	SortOrder     int               `json:"sort_order"`
	Timezone      string            `json:"timezone,omitempty"`
	Location      string            `json:"location,omitempty"`
	Latitude      *float64          `json:"latitude,omitempty"`
	Longitude     *float64          `json:"longitude,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	JumpChain     json.RawMessage   `json:"jump_chain,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// CreateRequest contains fields for creating a group.
type CreateRequest struct {
	Name          string            `json:"name"`
	Slug          string            `json:"slug"`
	ParentGroupID string            `json:"parent_group_id,omitempty"`
	Icon          string            `json:"icon,omitempty"`
	SortOrder     int               `json:"sort_order"`
	Timezone      string            `json:"timezone,omitempty"`
	Location      string            `json:"location,omitempty"`
	Latitude      *float64          `json:"latitude,omitempty"`
	Longitude     *float64          `json:"longitude,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	JumpChain     json.RawMessage   `json:"jump_chain,omitempty"`
}

// UpdateRequest contains mutable group fields. Pointer fields allow
// distinguishing between "not provided" and "set to zero/empty".
type UpdateRequest struct {
	Name          *string           `json:"name,omitempty"`
	Slug          *string           `json:"slug,omitempty"`
	ParentGroupID *string           `json:"parent_group_id,omitempty"`
	Icon          *string           `json:"icon,omitempty"`
	SortOrder     *int              `json:"sort_order,omitempty"`
	Timezone      *string           `json:"timezone,omitempty"`
	Location      *string           `json:"location,omitempty"`
	Latitude      *float64          `json:"latitude,omitempty"`
	Longitude     *float64          `json:"longitude,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	JumpChain     json.RawMessage   `json:"jump_chain,omitempty"`
}

// TreeNode represents a group within its hierarchical tree, including
// its children and depth level for rendering.
type TreeNode struct {
	Group    Group      `json:"group"`
	Children []TreeNode `json:"children,omitempty"`
	Depth    int        `json:"depth"`
}
