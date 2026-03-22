package pbs

import (
	"strings"
	"net/http"
	"sync"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/secrets"
)

// Deps holds all dependencies required by the PBS handler package.
type Deps struct {
	AssetStore        persistence.AssetStore
	HubCollectorStore persistence.HubCollectorStore
	CredentialStore   persistence.CredentialStore
	SecretsManager    *secrets.Manager

	PBSCacheMu sync.RWMutex
	PBSCache   map[string]*CachedPBSRuntime

	RequireAdminAuth func(w http.ResponseWriter, r *http.Request) bool

	WrapAuth  func(http.HandlerFunc) http.HandlerFunc
	WrapAdmin func(http.HandlerFunc) http.HandlerFunc
}

// RegisterRoutes registers all PBS API routes on the given mux.
func RegisterRoutes(mux *http.ServeMux, d *Deps) {
	mux.HandleFunc("/pbs/assets/", d.WrapAuth(d.HandlePBSAssets))
}

// DedupeNonEmptyWarnings returns a deduplicated list of non-empty warnings.
// Whitespace-only strings are filtered. Deduplication is case-insensitive
// but the original casing of the first occurrence is preserved.
func DedupeNonEmptyWarnings(warnings []string) []string {
	if len(warnings) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(warnings))
	out := make([]string, 0, len(warnings))
	for _, w := range warnings {
		trimmed := strings.TrimSpace(w)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
