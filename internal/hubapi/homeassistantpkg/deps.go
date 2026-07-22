package homeassistantpkg

import (
	"net/http"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/secrets"
)

// Deps holds the persisted inventory and encrypted runtime configuration used
// by the Home Assistant v2 API. Runtime credentials are decrypted only for the
// duration of the outbound request and are never returned by these handlers.
type Deps struct {
	AssetStore        persistence.AssetStore
	HubCollectorStore persistence.HubCollectorStore
	CredentialStore   persistence.CredentialStore
	SecretsManager    *secrets.Manager

	RequireAdminAuth func(http.ResponseWriter, *http.Request) bool
}
