package resources

// Asset validation limits — kept in parity with the same constants in
// cmd/labtether/main.go so that moved handlers compile without importing main.
const (
	MaxPlanNameLength = 120
	MaxAssetTagCount  = 32
	MaxAssetTagLength = 64
)
