package main

import (
	"context"
	"net/http"

	"github.com/labtether/labtether/internal/auth"
	authpkg "github.com/labtether/labtether/internal/hubapi/auth"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

// buildAuthDeps constructs the auth.Deps from the apiServer's fields.
func (s *apiServer) buildAuthDeps() *authpkg.Deps {
	return &authpkg.Deps{
		AuthStore:         s.authStore,
		OIDCRef:           s.oidcRef,
		SettingsStore:     s.db,
		TLSEnabled:        s.tlsState.Enabled,
		ChallengeStore:    s.challengeStore,
		TOTPEncryptionKey: s.totpEncryptionKey,

		// Credential and API key stores.
		CredentialStore: s.credentialStore,
		AssetStore:      s.assetStore,
		SecretsManager:  s.secretsManager,
		PolicyState:     s.policyState,
		APIKeyStore:     s.apiKeyStore,

		AppendAuditEventBestEffort: s.appendAuditEventBestEffort,
		EnforceRateLimit:           s.enforceRateLimit,
		ValidateOwnerTokenRequest:  s.validateOwnerTokenRequest,
		UserIDFromContext:          principalActorID,

		WrapAuth:  s.withAuth,
		WrapAdmin: s.withAdminAuth,
	}
}

// ensureAuthDeps returns authDeps, lazily building and caching on first call.
// The auth package owns OIDC state, so a stable instance is required.
func (s *apiServer) ensureAuthDeps() *authpkg.Deps {
	s.authDepsOnce.Do(func() {
		if s.authDeps == nil {
			s.authDeps = s.buildAuthDeps()
		}
	})
	return s.authDeps
}

// Forwarding methods from apiServer to auth.Deps so that existing
// cmd/labtether/ callers keep compiling without changes.

func (s *apiServer) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().HandleAuthLogin(w, r)
}

func (s *apiServer) handleLogin2FA(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().HandleLogin2FA(w, r)
}

func (s *apiServer) handleAuthBootstrapStatus(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().HandleAuthBootstrapStatus(w, r)
}

func (s *apiServer) handleAuthBootstrapSetup(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().HandleAuthBootstrapSetup(w, r)
}

func (s *apiServer) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().HandleAuthLogout(w, r)
}

func (s *apiServer) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().HandleAuthMe(w, r)
}

func (s *apiServer) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().HandleChangePassword(w, r)
}

func (s *apiServer) handleDeleteOwnAccount(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().HandleDeleteOwnAccount(w, r)
}

func (s *apiServer) handle2FASetup(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().Handle2FASetup(w, r)
}

func (s *apiServer) handle2FAVerify(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().Handle2FAVerify(w, r)
}

func (s *apiServer) handle2FADisable(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().Handle2FADisable(w, r)
}

func (s *apiServer) handle2FARecoveryCodes(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().Handle2FARecoveryCodes(w, r)
}

func (s *apiServer) handleAuthProviders(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().HandleAuthProviders(w, r)
}

func (s *apiServer) handleAuthOIDCStart(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().HandleAuthOIDCStart(w, r)
}

func (s *apiServer) handleAuthOIDCCallback(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().HandleAuthOIDCCallback(w, r)
}

func (s *apiServer) handleAuthUsers(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().HandleAuthUsers(w, r)
}

func (s *apiServer) handleAuthUserActions(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().HandleAuthUserActions(w, r)
}

// completeLogin delegates to the auth package.
func (s *apiServer) completeLogin(w http.ResponseWriter, user auth.User) {
	s.ensureAuthDeps().CompleteLogin(w, user)
}

// Package-level function aliases delegating to the auth package.

func bootstrapAdminUser(store authpkg.AdminBootstrapStore) error {
	return authpkg.BootstrapAdminUser(store)
}

func configuredBootstrapAdminUsername() (string, error) {
	return authpkg.ConfiguredBootstrapAdminUsername()
}

func isBootstrapAdminUsername(username string) bool {
	return authpkg.IsBootstrapAdminUsername(username)
}

func authBootstrapSetupRequired(store authpkg.AdminBootstrapStore) (bool, error) {
	return authpkg.AuthBootstrapSetupRequired(store)
}

func normalizeUsername(raw string) string {
	return authpkg.NormalizeUsername(raw)
}

func validateLoginRequest(req loginRequest) error {
	return authpkg.ValidateLoginRequest(authpkg.LoginRequest{Username: req.Username, Password: req.Password})
}

func sanitizeNextPath(next string) string {
	return authpkg.SanitizeNextPath(next)
}

func validateAuthRedirectURI(raw string) (string, error) {
	return authpkg.ValidateAuthRedirectURI(raw)
}

func randomURLToken(length int) (string, error) {
	return authpkg.RandomURLToken(length)
}

func runSessionCleanupLoop(ctx context.Context, store *persistence.PostgresStore) {
	authpkg.RunSessionCleanupLoop(ctx, store)
}

// Type aliases for auth types used in cmd/labtether/.
type loginRequest = authpkg.LoginRequest
type adminBootstrapStore = authpkg.AdminBootstrapStore

// Constants delegated to the auth package.
var (
	defaultBootstrapAdminUsername = authpkg.DefaultBootstrapAdminUsername
	weakPasswords                 = authpkg.WeakPasswords
	errOIDCSetupRequired          = authpkg.ErrOIDCSetupRequired
)

// oidcAssignableRole delegates to the auth package.
func oidcAssignableRole(role string) string {
	return authpkg.OIDCAssignableRole(role)
}

// storeOIDCState delegates to the auth deps.
func (s *apiServer) storeOIDCState(state string, entry oidcAuthState) bool {
	return s.ensureAuthDeps().StoreOIDCState(state, entry)
}

// consumeOIDCState delegates to the auth deps.
func (s *apiServer) consumeOIDCState(state, redirectURI string) (oidcAuthState, bool) {
	return s.ensureAuthDeps().ConsumeOIDCState(state, redirectURI)
}

// resolveOIDCUser delegates to the auth deps.
func (s *apiServer) resolveOIDCUser(identity auth.OIDCIdentity) (auth.User, bool, error) {
	return s.ensureAuthDeps().ResolveOIDCUser(identity)
}

// tryRecoveryCode delegates to the auth deps.
func (s *apiServer) tryRecoveryCode(user auth.User, code string) bool {
	return s.ensureAuthDeps().TryRecoveryCode(user, code)
}

func (s *apiServer) handleOIDCSettingsGet(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().HandleOIDCSettingsGet(w, r)
}

func (s *apiServer) handleOIDCSettingsPut(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().HandleOIDCSettingsPut(w, r)
}

func (s *apiServer) handleOIDCSettingsApply(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().HandleOIDCSettingsApply(w, r)
}

func (s *apiServer) handleOIDCSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleOIDCSettingsGet(w, r)
	case http.MethodPut:
		s.handleOIDCSettingsPut(w, r)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// Credential handler forwarding.

func (s *apiServer) handleCredentialProfiles(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().HandleCredentialProfiles(w, r)
}

func (s *apiServer) handleCredentialProfileActions(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().HandleCredentialProfileActions(w, r)
}

func (s *apiServer) handleDesktopCredentials(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().HandleDesktopCredentials(w, r)
}

func (s *apiServer) handleRetrieveDesktopCredentials(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().HandleRetrieveDesktopCredentials(w, r)
}

// API key handler forwarding.

func (s *apiServer) handleAPIKeys(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().HandleAPIKeys(w, r)
}

func (s *apiServer) handleAPIKeyActions(w http.ResponseWriter, r *http.Request) {
	s.ensureAuthDeps().HandleAPIKeyActions(w, r)
}
