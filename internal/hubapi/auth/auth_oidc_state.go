package auth

import (
	"strings"
	"time"
)

const maxPendingOIDCStates = 256

// StoreOIDCState stores a pending OIDC authentication state entry.
func (d *Deps) StoreOIDCState(state string, entry OIDCAuthState) bool {
	if d == nil {
		return false
	}
	state = strings.TrimSpace(state)
	if state == "" {
		return false
	}
	now := time.Now().UTC()

	d.OIDCStateMu.Lock()
	defer d.OIDCStateMu.Unlock()
	if d.OIDCStates == nil {
		d.OIDCStates = make(map[string]OIDCAuthState, 128)
	}
	for key, value := range d.OIDCStates {
		if value.ExpiresAt.Before(now) {
			delete(d.OIDCStates, key)
		}
	}
	if len(d.OIDCStates) >= maxPendingOIDCStates {
		return false
	}
	d.OIDCStates[state] = entry
	return true
}

// ConsumeOIDCState atomically retrieves and removes a pending OIDC state entry.
func (d *Deps) ConsumeOIDCState(state, redirectURI string) (OIDCAuthState, bool) {
	return d.consumeOIDCStateForFlow(state, redirectURI, OIDCAuthFlowWeb)
}

// ConsumeMobileOIDCState consumes only state created by the native PKCE start
// endpoint. It cannot consume a web login attempt, even if the caller somehow
// obtains its state value.
func (d *Deps) ConsumeMobileOIDCState(state, redirectURI string) (OIDCAuthState, bool) {
	return d.consumeOIDCStateForFlow(state, redirectURI, OIDCAuthFlowMobile)
}

func (d *Deps) consumeOIDCStateForFlow(state, redirectURI string, expectedFlow OIDCAuthFlow) (OIDCAuthState, bool) {
	if d == nil {
		return OIDCAuthState{}, false
	}
	state = strings.TrimSpace(state)
	redirectURI = strings.TrimSpace(redirectURI)
	if state == "" || redirectURI == "" {
		return OIDCAuthState{}, false
	}
	now := time.Now().UTC()

	d.OIDCStateMu.Lock()
	defer d.OIDCStateMu.Unlock()
	entry, ok := d.OIDCStates[state]
	if !ok {
		return OIDCAuthState{}, false
	}
	delete(d.OIDCStates, state)
	if entry.ExpiresAt.Before(now) {
		return OIDCAuthState{}, false
	}
	if !strings.EqualFold(strings.TrimSpace(entry.RedirectURI), redirectURI) {
		return OIDCAuthState{}, false
	}
	if expectedFlow == OIDCAuthFlowWeb {
		// Empty is accepted for backward compatibility with state created before
		// flows were tagged; all new web state is tagged explicitly.
		if entry.Flow != "" && entry.Flow != OIDCAuthFlowWeb {
			return OIDCAuthState{}, false
		}
	} else if entry.Flow != expectedFlow {
		return OIDCAuthState{}, false
	}
	return entry, true
}
