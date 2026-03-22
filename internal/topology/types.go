package topology

import "time"

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Size struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type Viewport struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Zoom float64 `json:"zoom"`
}

type Layout struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Viewport  Viewport  `json:"viewport"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Zone struct {
	ID           string   `json:"id"`
	TopologyID   string   `json:"topology_id"`
	ParentZoneID string   `json:"parent_zone_id,omitempty"`
	Label        string   `json:"label"`
	Color        string   `json:"color"`
	Icon         string   `json:"icon,omitempty"`
	Position     Position `json:"position"`
	Size         Size     `json:"size"`
	Collapsed    bool     `json:"collapsed"`
	SortOrder    int      `json:"sort_order"`
}

type ZoneMember struct {
	ZoneID    string   `json:"zone_id"`
	AssetID   string   `json:"asset_id"`
	Position  Position `json:"position"`
	SortOrder int      `json:"sort_order"`
}

// ValidRelationships is the set of allowed relationship types for topology connections.
var ValidRelationships = map[string]bool{
	"runs_on":      true,
	"hosted_on":    true,
	"depends_on":   true,
	"provides_to":  true,
	"connected_to": true,
	"peer_of":      true,
}

type Connection struct {
	ID            string `json:"id"`
	TopologyID    string `json:"topology_id"`
	SourceAssetID string `json:"source_asset_id"`
	TargetAssetID string `json:"target_asset_id"`
	Relationship  string `json:"relationship"`
	UserDefined   bool   `json:"user_defined"`
	Label         string `json:"label,omitempty"`
	Deleted       bool   `json:"-"`
}

// MergedConnection is the result of merging topology_connections with asset_edges.
type MergedConnection struct {
	Connection
	Origin string `json:"origin"` // "discovered", "user", "accepted"
}

type TopologyState struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Zones       []Zone             `json:"zones"`
	Members     []ZoneMember       `json:"members"`
	Connections []MergedConnection `json:"connections"`
	Unsorted    []string           `json:"unsorted"`
	Viewport    Viewport           `json:"viewport"`
}
