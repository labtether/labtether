package topology

// Store defines persistence operations for the topology canvas.
type Store interface {
	// Layout
	GetOrCreateLayout() (Layout, error)
	UpdateViewport(viewport Viewport) error

	// Zones
	CreateZone(z Zone) (Zone, error)
	UpdateZone(z Zone) error
	DeleteZone(id string) error
	ListZones(topologyID string) ([]Zone, error)
	ReorderZones(updates []ZoneReorder) error

	// Members
	SetMembers(zoneID string, members []ZoneMember) error
	RemoveMember(assetID string) error
	ListMembers(topologyID string) ([]ZoneMember, error)

	// Connections
	CreateConnection(c Connection) (Connection, error)
	UpdateConnection(id string, relationship, label string) error
	DeleteConnection(id string) error
	ListConnections(topologyID string) ([]Connection, error)

	// Dismissed
	DismissAsset(topologyID, assetID string) error
	UndismissAsset(topologyID, assetID string) error
	ListDismissed(topologyID string) ([]string, error)

	// Reset
	ClearTopology(topologyID string) error
}

type ZoneReorder struct {
	ZoneID       string `json:"zone_id"`
	ParentZoneID string `json:"parent_zone_id,omitempty"`
	SortOrder    int    `json:"sort_order"`
}
