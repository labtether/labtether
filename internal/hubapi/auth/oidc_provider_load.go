package auth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/auth"
)

const (
	oidcProviderLoadTimeout = 15 * time.Second
	oidcStartRetryInitial   = 30 * time.Second
	oidcStartRetryMax       = 5 * time.Minute
)

var (
	errOIDCSettingsUnavailable     = errors.New("failed to load oidc settings")
	errOIDCClientSecretUnavailable = errors.New("failed to load oidc client secret")
)

// LoadEffectiveOIDCProvider builds the single sign-on provider from the
// settings saved in the console, with LABTETHER_OIDC_* environment values
// taking precedence. Applying settings and starting the Hub both use it. A
// nil provider with a nil error means single sign-on is off.
func (d *Deps) LoadEffectiveOIDCProvider(ctx context.Context) (*auth.OIDCProvider, bool, error) {
	db, err := d.loadOIDCDBSettings(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %v", errOIDCSettingsUnavailable, err)
	}
	dbSecretConfigured := storedOIDCClientSecretConfigured(db)
	merged, _ := mergeOIDCSettings(db, "", dbSecretConfigured)
	if merged.Enabled && strings.TrimSpace(os.Getenv("LABTETHER_OIDC_CLIENT_SECRET")) == "" && dbSecretConfigured {
		dbClientSecret, decryptErr := d.decryptOIDCClientSecret(db)
		if decryptErr != nil {
			return nil, false, fmt.Errorf("%w: %v", errOIDCClientSecretUnavailable, decryptErr)
		}
		merged, _ = mergeOIDCSettings(db, dbClientSecret, true)
	}
	autoProvision := true
	if db.AutoProvision != nil {
		autoProvision = *db.AutoProvision
	}
	autoProvision = envOrDefaultBool("LABTETHER_OIDC_AUTO_PROVISION", autoProvision)

	provider, err := auth.NewOIDCProvider(ctx, merged)
	if err != nil {
		return nil, autoProvision, err
	}
	return provider, autoProvision, nil
}

// ActivateOIDCAtStart turns on the single sign-on settings saved in the
// console (or set in the environment) when the Hub starts. When the identity
// provider does not answer, the Hub starts without single sign-on, logs one
// line and keeps trying in the background, until it answers or an admin
// applies settings.
func (d *Deps) ActivateOIDCAtStart(ctx context.Context) {
	version := d.OIDCRef.Version()
	if !d.activateOIDCOnce(ctx, version, true) {
		return
	}
	go func() {
		delay := oidcStartRetryInitial
		for {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if !d.activateOIDCOnce(ctx, version, false) {
				return
			}
			delay = min(delay*2, oidcStartRetryMax)
		}
	}()
}

// activateOIDCOnce makes one attempt and reports whether to try again.
func (d *Deps) activateOIDCOnce(ctx context.Context, version uint64, first bool) bool {
	attemptCtx, cancel := context.WithTimeout(ctx, oidcProviderLoadTimeout)
	defer cancel()
	provider, autoProvision, err := d.LoadEffectiveOIDCProvider(attemptCtx)
	switch {
	case err == nil:
		if d.OIDCRef.SwapIfVersion(version, provider, autoProvision) && provider != nil {
			log.Printf("labtether auth: single sign-on is on (issuer=%s, auto_provision=%t)", provider.IssuerURL(), autoProvision)
		}
		return false
	case d.OIDCRef.Version() != version || ctx.Err() != nil:
		return false
	case errors.Is(err, auth.ErrOIDCProviderUnavailable):
		if first {
			log.Printf("labtether auth: single sign-on is off until the identity provider answers; the Hub keeps trying in the background: %v", err)
		}
		return true
	default:
		log.Printf("labtether auth: single sign-on is off: %v. Correct it under Security > OIDC / SSO and apply.", err)
		return false
	}
}

// splitOIDCEnvList splits an environment list on commas, semicolons or
// whitespace, so LABTETHER_OIDC_SCOPES="openid profile email" also works.
func splitOIDCEnvList(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
}
