package main

import (
	whoamipkg "github.com/labtether/labtether/internal/hubapi/whoamipkg"
)

// buildWhoamiDeps constructs the whoamipkg.Deps from the apiServer's fields.
func (s *apiServer) buildWhoamiDeps() *whoamipkg.Deps {
	return &whoamipkg.Deps{
		AssetStore:  s.assetStore,
		APIKeyStore: s.apiKeyStore,
	}
}

// ensureWhoamiDeps returns the whoami deps, creating and caching on first call.
func (s *apiServer) ensureWhoamiDeps() *whoamipkg.Deps {
	s.whoamiDepsOnce.Do(func() {
		s.whoamiDeps = s.buildWhoamiDeps()
	})
	return s.whoamiDeps
}
