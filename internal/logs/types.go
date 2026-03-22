package logs

import "time"

// Event is a normalized log record in LabTether.
type Event struct {
	ID        string            `json:"id"`
	AssetID   string            `json:"asset_id,omitempty"`
	Source    string            `json:"source"`
	Level     string            `json:"level"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// DeadLetterEvent is a projected dead-letter log row shape optimized for
// dead-letter list/analytics query paths.
type DeadLetterEvent struct {
	ID         string
	Component  string
	Subject    string
	Deliveries uint64
	Error      string
	PayloadB64 string
	CreatedAt  time.Time
}

// QueryRequest defines log query filters.
type QueryRequest struct {
	AssetID       string
	Source        string
	Level         string
	Search        string
	GroupID       string
	GroupAssetIDs []string
	From          time.Time
	To            time.Time
	Limit         int
	ExcludeFields bool
	FieldKeys     []string
}

// SourceSummary is an aggregate row for log sources.
type SourceSummary struct {
	Source     string    `json:"source"`
	Count      int       `json:"count"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

// SavedViewRequest captures persisted log filter preferences.
type SavedViewRequest struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name"`
	AssetID string `json:"asset_id,omitempty"`
	Source  string `json:"source,omitempty"`
	Level   string `json:"level,omitempty"`
	Search  string `json:"search,omitempty"`
	Window  string `json:"window,omitempty"`
}

// SavedView is a persisted log filter profile.
type SavedView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	AssetID   string    `json:"asset_id,omitempty"`
	Source    string    `json:"source,omitempty"`
	Level     string    `json:"level,omitempty"`
	Search    string    `json:"search,omitempty"`
	Window    string    `json:"window,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
