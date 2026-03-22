package savedactions

import "time"

// SavedAction represents a reusable named command sequence stored in the hub.
type SavedAction struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Steps       []ActionStep `json:"steps"`
	CreatedBy   string       `json:"created_by"`
	CreatedAt   time.Time    `json:"created_at"`
}

// ActionStep is a single command within a SavedAction.
type ActionStep struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Target  string `json:"target"`
}

// CreateRequest is the API request body for creating a saved action.
type CreateRequest struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Steps       []ActionStep `json:"steps"`
}
