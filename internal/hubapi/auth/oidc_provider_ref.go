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
	// version counts swaps, so a background start-up retry can tell that an
	// admin applied settings in the meantime.
	version uint64
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
	r.version++
}

// Version returns how many times the provider was swapped.
func (r *OIDCProviderRef) Version() uint64 {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.version
}

// SwapIfVersion swaps only when no other swap happened since version was
// read, and reports whether it swapped.
func (r *OIDCProviderRef) SwapIfVersion(version uint64, provider *auth.OIDCProvider, autoProvision bool) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.version != version {
		return false
	}
	r.provider = provider
	r.autoProvision = autoProvision
	r.version++
	return true
}
