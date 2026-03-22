package auth

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/policy"
)

// CredentialSecretsManager is the subset of secrets.Manager used by
// credential handlers in this package.
type CredentialSecretsManager interface {
	EncryptString(plaintext, aad string) (string, error)
	DecryptString(ciphertext, aad string) (string, error)
}

// PolicyStateReader is the subset of operations.PolicyRuntimeState used by
// credential authorization.
type PolicyStateReader interface {
	Current() policy.EvaluatorConfig
}

// Deps holds all dependencies required by the auth handler package.
// Store interfaces are embedded directly; cross-cutting concerns that live
// in other cmd/labtether subsystems are injected as function fields.
type Deps struct {
	// Store interfaces
	AuthStore persistence.AuthStore

	// OIDC provider ref (nil when OIDC is disabled).
	OIDCRef       *OIDCProviderRef
	SettingsStore persistence.SettingsStore

	// Credential stores — used by credential and desktop-credential handlers.
	CredentialStore persistence.CredentialStore
	AssetStore      persistence.AssetStore
	SecretsManager  CredentialSecretsManager

	// PolicyState is used to authorize desktop asset access.
	PolicyState PolicyStateReader

	// APIKeyStore — used by API key management handlers.
	APIKeyStore persistence.APIKeyStore

	// AppendAuditEventBestEffort appends an audit event, logging on failure.
	AppendAuditEventBestEffort func(event audit.Event, logMessage string)

	// TLS state (for setting Secure on cookies).
	TLSEnabled bool

	// 2FA challenge store + TOTP encryption key.
	ChallengeStore    *auth.ChallengeStore
	TOTPEncryptionKey []byte

	// OIDC state management (owned by this package after extraction).
	OIDCStateMu sync.Mutex
	OIDCStates  map[string]OIDCAuthState

	// Auth middleware injected from cmd/labtether.
	EnforceRateLimit          func(w http.ResponseWriter, r *http.Request, bucket string, limit int, window time.Duration) bool
	ValidateOwnerTokenRequest func(r *http.Request) bool

	// Context extractors injected from cmd/labtether.
	UserIDFromContext func(ctx context.Context) string

	// WrapAuth / WrapAdmin for route registration.
	WrapAuth  func(http.HandlerFunc) http.HandlerFunc
	WrapAdmin func(http.HandlerFunc) http.HandlerFunc
}

// OIDCAuthState holds the pending OIDC authentication state.
type OIDCAuthState struct {
	Nonce       string
	NextPath    string
	RedirectURI string
	ExpiresAt   time.Time
}

// RegisterRoutes registers all auth API routes on the given mux.
func RegisterRoutes(mux *http.ServeMux, d *Deps) {
	mux.HandleFunc("/auth/login", d.HandleAuthLogin)
	mux.HandleFunc("/auth/login/2fa", d.HandleLogin2FA)
	mux.HandleFunc("/auth/bootstrap/status", d.HandleAuthBootstrapStatus)
	mux.HandleFunc("/auth/bootstrap", d.HandleAuthBootstrapSetup)
	mux.HandleFunc("/auth/logout", d.HandleAuthLogout)
	mux.HandleFunc("/auth/me", d.WrapAuth(d.HandleAuthMe))
	mux.HandleFunc("/auth/me/password", d.WrapAuth(d.HandleChangePassword))
	mux.HandleFunc("/auth/account", d.WrapAuth(d.HandleDeleteOwnAccount))
	mux.HandleFunc("/auth/2fa/setup", d.WrapAuth(d.Handle2FASetup))
	mux.HandleFunc("/auth/2fa/verify", d.WrapAuth(d.Handle2FAVerify))
	mux.HandleFunc("/auth/2fa", d.WrapAuth(d.Handle2FADisable))
	mux.HandleFunc("/auth/2fa/recovery-codes", d.WrapAuth(d.Handle2FARecoveryCodes))
	mux.HandleFunc("/auth/providers", d.HandleAuthProviders)
	mux.HandleFunc("/auth/oidc/start", d.HandleAuthOIDCStart)
	mux.HandleFunc("/auth/oidc/callback", d.HandleAuthOIDCCallback)
	mux.HandleFunc("/auth/users", d.WrapAdmin(d.HandleAuthUsers))
	mux.HandleFunc("/auth/users/", d.WrapAdmin(d.HandleAuthUserActions))
}
