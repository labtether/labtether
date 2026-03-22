package auth

import (
	"sync"

	"github.com/labtether/labtether/internal/auth"
)

// OIDCProviderRef holds an OIDC provider reference that can be atomically
// swapped at runtime (e.g., when an admin applies new OIDC settings).
type OIDCProviderRef struct {
	mu            sync.RWMutex
	provider      *auth.OIDCProvider
	autoProvision bool
}

// NewOIDCProviderRef creates a new ref with the given initial state.
func NewOIDCProviderRef(provider *auth.OIDCProvider, autoProvision bool) *OIDCProviderRef {
	return &OIDCProviderRef{
		provider:      provider,
		autoProvision: autoProvision,
	}
}

// Get returns the current provider and auto-provision flag.
// Safe to call on a nil receiver; returns (nil, false) in that case.
func (r *OIDCProviderRef) Get() (*auth.OIDCProvider, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.provider, r.autoProvision
}

// Swap atomically replaces the provider and auto-provision flag.
func (r *OIDCProviderRef) Swap(provider *auth.OIDCProvider, autoProvision bool) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.provider = provider
	r.autoProvision = autoProvision
}
