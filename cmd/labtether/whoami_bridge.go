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
	if s.whoamiDeps != nil {
		return s.whoamiDeps
	}
	d := s.buildWhoamiDeps()
	s.whoamiDeps = d
	return d
}
