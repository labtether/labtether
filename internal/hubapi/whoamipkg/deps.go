package whoamipkg

import (
	"github.com/labtether/labtether/internal/persistence"
)

// Deps holds the dependencies for the whoami handler.
type Deps struct {
	AssetStore  persistence.AssetStore
	APIKeyStore persistence.APIKeyStore
}
