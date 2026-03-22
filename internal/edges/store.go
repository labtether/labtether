package edges

// Store defines persistence operations for edges and composites.
type Store interface {
	// Edge CRUD
	CreateEdge(req CreateEdgeRequest) (Edge, error)
	GetEdge(id string) (Edge, bool, error)
	UpdateEdge(id string, relType, criticality string) error
	DeleteEdge(id string) error

	// Edge queries
	ListEdgesByAsset(assetID string, limit int) ([]Edge, error)
	ListEdgesBatch(assetIDs []string, limit int) ([]Edge, error)

	// Graph traversal
	Descendants(rootAssetID string, maxDepth int) ([]TreeNode, error)
	Ancestors(assetID string, maxDepth int) ([]TreeNode, error)

	// Proposals (edges with origin='suggested')
	ListProposals() ([]Edge, error)
	AcceptProposal(edgeID string) error
	DismissProposal(edgeID string) error

	// Composite CRUD
	CreateComposite(req CreateCompositeRequest) (Composite, error)
	GetComposite(compositeID string) (Composite, bool, error)
	ChangePrimary(compositeID, newPrimaryAssetID string) error
	DetachMember(compositeID, memberAssetID string) error
	ListCompositesByAssets(assetIDs []string) ([]Composite, error)

	// Composite query
	ResolveCompositeID(assetID string) (compositeID string, found bool, err error)
}
