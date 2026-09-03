package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/labtether/labtether/internal/securityruntime"
	"golang.org/x/oauth2"
)

const PKCECodeChallengeMethodS256 = "S256"

var (
	ErrOIDCInvalidConfiguration = errors.New("invalid oidc configuration")
	ErrOIDCProviderUnavailable  = errors.New("oidc provider unavailable")
)

type OIDCSettings struct {
	Enabled            bool
	IssuerURL          string
	ClientID           string
	ClientSecret       string // #nosec G117 -- Runtime OIDC credential, not a hardcoded secret.
	Scopes             []string
	RoleClaim          string
	DefaultRole        string
	DisplayName        string
	AdminRoleValues    []string
	OperatorRoleValues []string
	// AllowedEndpointOrigins is deployment-only. It permits exact, additional
	// origins for providers whose token or signing-key endpoints differ from
	// the issuer origin.
	AllowedEndpointOrigins []string
	// These deployment-only switches are a strict ceiling on the shared
	// outbound policy. Link-local OIDC endpoints are never allowed.
	AllowPrivateEndpoints  bool
	AllowLoopbackEndpoints bool
}

type OIDCIdentity struct {
	Issuer            string         `json:"issuer"`
	Subject           string         `json:"subject"`
	Email             string         `json:"email,omitempty"`
	Name              string         `json:"name,omitempty"`
	PreferredUsername string         `json:"preferred_username,omitempty"`
	Role              string         `json:"role"`
	Claims            map[string]any `json:"claims,omitempty"`
}

type OIDCProvider struct {
	settings   OIDCSettings
	provider   *oidc.Provider
	verifier   *oidc.IDTokenVerifier
	httpClient *http.Client
}

func NewOIDCProvider(ctx context.Context, settings OIDCSettings) (*OIDCProvider, error) {
	if !settings.Enabled {
		return nil, nil
	}
	settings.IssuerURL = strings.TrimSpace(settings.IssuerURL)
	settings.ClientID = strings.TrimSpace(settings.ClientID)
	settings.ClientSecret = strings.TrimSpace(settings.ClientSecret)
	if settings.IssuerURL == "" {
		return nil, fmt.Errorf("%w: issuer URL is required", ErrOIDCInvalidConfiguration)
	}
	if settings.ClientID == "" {
		return nil, fmt.Errorf("%w: client ID is required", ErrOIDCInvalidConfiguration)
	}
	if settings.ClientSecret == "" {
		return nil, fmt.Errorf("%w: client secret is required", ErrOIDCInvalidConfiguration)
	}
	issuerURL, err := validateOIDCIssuerURL(settings.IssuerURL)
	if err != nil {
		return nil, err
	}
	settings.IssuerURL = issuerURL
	if len(settings.Scopes) == 0 {
		settings.Scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}
	settings.RoleClaim = strings.TrimSpace(settings.RoleClaim)
	if settings.RoleClaim == "" {
		settings.RoleClaim = "labtether_role"
	}
	settings.DefaultRole = NormalizeRole(settings.DefaultRole)
	if settings.DefaultRole == "" || settings.DefaultRole == RoleOwner {
		settings.DefaultRole = RoleViewer
	}
	if strings.TrimSpace(settings.DisplayName) == "" {
		settings.DisplayName = "OIDC"
	}

	allowedURLs, err := oidcAllowedEndpointURLs(settings)
	if err != nil {
		return nil, err
	}
	addressPolicy := oidcOutboundAddressPolicy(settings)
	httpClient, err := securityruntime.NewOriginRestrictedOutboundHTTPClient(nil, addressPolicy, allowedURLs...)
	if err != nil {
		return nil, fmt.Errorf("%w: issuer or allowed endpoint origin: %v", ErrOIDCInvalidConfiguration, err)
	}
	oidcContext := oidc.ClientContext(ctx, httpClient)
	provider, err := oidc.NewProvider(oidcContext, settings.IssuerURL)
	if err != nil {
		return nil, ErrOIDCProviderUnavailable
	}
	if err := validateOIDCProviderEndpoints(provider, addressPolicy, allowedURLs); err != nil {
		return nil, err
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: settings.ClientID})

	return &OIDCProvider{
		settings:   settings,
		provider:   provider,
		verifier:   verifier,
		httpClient: httpClient,
	}, nil
}

func validateOIDCIssuerURL(rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	parsed, err := url.ParseRequestURI(trimmed)
	if err != nil || !parsed.IsAbs() || parsed.Hostname() == "" {
		return "", fmt.Errorf("%w: issuer URL must be absolute", ErrOIDCInvalidConfiguration)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", fmt.Errorf("%w: issuer URL must use HTTP or HTTPS", ErrOIDCInvalidConfiguration)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", fmt.Errorf("%w: issuer URL must not contain userinfo, a query, or a fragment", ErrOIDCInvalidConfiguration)
	}
	return parsed.String(), nil
}

func oidcAllowedEndpointURLs(settings OIDCSettings) ([]string, error) {
	issuer, err := url.Parse(settings.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid issuer URL", ErrOIDCInvalidConfiguration)
	}
	allowed := []string{settings.IssuerURL}
	for _, rawOrigin := range settings.AllowedEndpointOrigins {
		rawOrigin = strings.TrimSpace(rawOrigin)
		if rawOrigin == "" {
			continue
		}
		parsed, err := url.ParseRequestURI(rawOrigin)
		if err != nil || !parsed.IsAbs() || parsed.Hostname() == "" {
			return nil, fmt.Errorf("%w: allowed endpoint origin must be absolute", ErrOIDCInvalidConfiguration)
		}
		parsed.Scheme = strings.ToLower(parsed.Scheme)
		if parsed.Scheme != strings.ToLower(issuer.Scheme) {
			return nil, fmt.Errorf("%w: allowed endpoint origins must use the issuer scheme", ErrOIDCInvalidConfiguration)
		}
		if parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
			return nil, fmt.Errorf("%w: allowed endpoint origins must not contain userinfo, paths, queries, or fragments", ErrOIDCInvalidConfiguration)
		}
		allowed = append(allowed, rawOrigin)
	}
	return allowed, nil
}

func oidcOutboundAddressPolicy(settings OIDCSettings) securityruntime.OutboundAddressPolicy {
	return securityruntime.OutboundAddressPolicy{
		AllowPrivate:   settings.AllowPrivateEndpoints,
		AllowLoopback:  settings.AllowLoopbackEndpoints,
		AllowLinkLocal: false,
	}
}

func validateOIDCProviderEndpoints(provider *oidc.Provider, addressPolicy securityruntime.OutboundAddressPolicy, allowedURLs []string) error {
	if provider == nil {
		return ErrOIDCProviderUnavailable
	}
	endpoints := provider.Endpoint()
	var metadata struct {
		JWKSURL string `json:"jwks_uri"`
	}
	if err := provider.Claims(&metadata); err != nil {
		return ErrOIDCProviderUnavailable
	}
	for name, rawURL := range map[string]string{
		"token":       endpoints.TokenURL,
		"signing key": metadata.JWKSURL,
	} {
		if strings.TrimSpace(rawURL) == "" {
			return fmt.Errorf("%w: %s endpoint is required", ErrOIDCInvalidConfiguration, name)
		}
		if err := securityruntime.ValidateOutboundURLAgainstOrigins(rawURL, addressPolicy, allowedURLs...); err != nil {
			return fmt.Errorf("%w: %s endpoint: %v", ErrOIDCInvalidConfiguration, name, err)
		}
	}
	return nil
}

func (p *OIDCProvider) Enabled() bool {
	return p != nil && p.provider != nil && p.verifier != nil
}

func (p *OIDCProvider) DisplayName() string {
	if p == nil {
		return ""
	}
	return p.settings.DisplayName
}

func (p *OIDCProvider) IssuerURL() string {
	if p == nil {
		return ""
	}
	return p.settings.IssuerURL
}

func (p *OIDCProvider) BuildAuthURL(state, nonce, redirectURI string) (string, error) {
	if !p.Enabled() {
		return "", errors.New("oidc is not enabled")
	}
	state = strings.TrimSpace(state)
	nonce = strings.TrimSpace(nonce)
	redirectURI = strings.TrimSpace(redirectURI)
	if state == "" {
		return "", errors.New("state is required")
	}
	if nonce == "" {
		return "", errors.New("nonce is required")
	}
	if redirectURI == "" {
		return "", errors.New("redirect uri is required")
	}

	cfg := oauth2.Config{
		ClientID:     p.settings.ClientID,
		ClientSecret: p.settings.ClientSecret,
		Endpoint:     p.provider.Endpoint(),
		RedirectURL:  redirectURI,
		Scopes:       p.settings.Scopes,
	}
	return cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("nonce", nonce)), nil
}

// BuildAuthURLWithPKCE builds an OIDC authorization URL bound to an RFC 7636
// S256 code challenge. Native clients keep the corresponding verifier local
// until the one-time callback exchange.
func (p *OIDCProvider) BuildAuthURLWithPKCE(state, nonce, redirectURI, codeChallenge string) (string, error) {
	if !p.Enabled() {
		return "", errors.New("oidc is not enabled")
	}
	state = strings.TrimSpace(state)
	nonce = strings.TrimSpace(nonce)
	redirectURI = strings.TrimSpace(redirectURI)
	codeChallenge = strings.TrimSpace(codeChallenge)
	if state == "" {
		return "", errors.New("state is required")
	}
	if nonce == "" {
		return "", errors.New("nonce is required")
	}
	if redirectURI == "" {
		return "", errors.New("redirect uri is required")
	}
	if err := ValidatePKCECodeChallenge(codeChallenge); err != nil {
		return "", err
	}

	cfg := oauth2.Config{
		ClientID:     p.settings.ClientID,
		ClientSecret: p.settings.ClientSecret,
		Endpoint:     p.provider.Endpoint(),
		RedirectURL:  redirectURI,
		Scopes:       p.settings.Scopes,
	}
	return cfg.AuthCodeURL(
		state,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", PKCECodeChallengeMethodS256),
	), nil
}

func (p *OIDCProvider) ExchangeCode(ctx context.Context, code, nonce, redirectURI string) (OIDCIdentity, error) {
	return p.exchangeCode(ctx, code, nonce, redirectURI)
}

// ExchangeCodeWithPKCE exchanges and verifies a native authorization code
// while proving possession of the verifier used to create its S256 challenge.
func (p *OIDCProvider) ExchangeCodeWithPKCE(ctx context.Context, code, nonce, redirectURI, codeVerifier string) (OIDCIdentity, error) {
	if err := ValidatePKCECodeVerifier(codeVerifier); err != nil {
		return OIDCIdentity{}, err
	}
	return p.exchangeCode(ctx, code, nonce, redirectURI, oauth2.VerifierOption(codeVerifier))
}

func (p *OIDCProvider) exchangeCode(ctx context.Context, code, nonce, redirectURI string, options ...oauth2.AuthCodeOption) (OIDCIdentity, error) {
	if !p.Enabled() {
		return OIDCIdentity{}, errors.New("oidc is not enabled")
	}
	code = strings.TrimSpace(code)
	nonce = strings.TrimSpace(nonce)
	redirectURI = strings.TrimSpace(redirectURI)
	if code == "" {
		return OIDCIdentity{}, errors.New("authorization code is required")
	}
	if nonce == "" {
		return OIDCIdentity{}, errors.New("nonce is required")
	}
	if redirectURI == "" {
		return OIDCIdentity{}, errors.New("redirect uri is required")
	}

	cfg := oauth2.Config{
		ClientID:     p.settings.ClientID,
		ClientSecret: p.settings.ClientSecret,
		Endpoint:     p.provider.Endpoint(),
		RedirectURL:  redirectURI,
		Scopes:       p.settings.Scopes,
	}
	exchangeContext := oidc.ClientContext(ctx, p.httpClient)
	token, err := cfg.Exchange(exchangeContext, code, options...)
	if err != nil {
		return OIDCIdentity{}, fmt.Errorf("exchange oidc code: %w", err)
	}
	idTokenRaw, ok := token.Extra("id_token").(string)
	if !ok || strings.TrimSpace(idTokenRaw) == "" {
		return OIDCIdentity{}, errors.New("oidc id_token missing from token response")
	}

	idToken, err := p.verifier.Verify(ctx, idTokenRaw)
	if err != nil {
		return OIDCIdentity{}, fmt.Errorf("verify oidc id_token: %w", err)
	}

	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		return OIDCIdentity{}, fmt.Errorf("decode oidc claims: %w", err)
	}
	if tokenNonce, _ := claims["nonce"].(string); strings.TrimSpace(tokenNonce) != nonce {
		return OIDCIdentity{}, errors.New("oidc nonce mismatch")
	}

	identity := OIDCIdentity{
		Issuer:            strings.TrimSpace(idToken.Issuer),
		Subject:           strings.TrimSpace(idToken.Subject),
		Email:             claimString(claims, "email"),
		Name:              claimString(claims, "name"),
		PreferredUsername: claimString(claims, "preferred_username"),
		Claims:            claims,
		Role:              p.resolveRole(claims),
	}
	if identity.Issuer == "" {
		return OIDCIdentity{}, errors.New("oidc issuer claim is missing")
	}
	if identity.Subject == "" {
		return OIDCIdentity{}, errors.New("oidc subject claim is missing")
	}
	return identity, nil
}

// ValidatePKCECodeChallenge accepts only a canonical SHA-256 base64url value.
func ValidatePKCECodeChallenge(codeChallenge string) error {
	if len(codeChallenge) != 43 {
		return errors.New("pkce code challenge must be a 43-character S256 value")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(codeChallenge)
	if err != nil || len(decoded) != sha256.Size || base64.RawURLEncoding.EncodeToString(decoded) != codeChallenge {
		return errors.New("pkce code challenge must be canonical base64url")
	}
	return nil
}

// ValidatePKCECodeVerifier applies RFC 7636's length and unreserved-character
// requirements. The value is deliberately not trimmed: any mutation would
// change its proof value.
func ValidatePKCECodeVerifier(codeVerifier string) error {
	if len(codeVerifier) < 43 || len(codeVerifier) > 128 {
		return errors.New("pkce code verifier must contain 43 to 128 characters")
	}
	for _, value := range []byte(codeVerifier) {
		if (value >= 'A' && value <= 'Z') ||
			(value >= 'a' && value <= 'z') ||
			(value >= '0' && value <= '9') ||
			value == '-' || value == '.' || value == '_' || value == '~' {
			continue
		}
		return errors.New("pkce code verifier contains an invalid character")
	}
	return nil
}

// PKCECodeChallengeS256 returns the canonical S256 challenge for a verifier.
func PKCECodeChallengeS256(codeVerifier string) (string, error) {
	if err := ValidatePKCECodeVerifier(codeVerifier); err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(codeVerifier))
	return base64.RawURLEncoding.EncodeToString(digest[:]), nil
}

func (p *OIDCProvider) resolveRole(claims map[string]any) string {
	values := claimValues(claims[p.settings.RoleClaim])
	if len(values) == 0 {
		values = claimValues(claims["groups"])
	}
	if len(values) == 0 {
		return p.settings.DefaultRole
	}

	adminSet := normalizeRoleSet(p.settings.AdminRoleValues)
	operatorSet := normalizeRoleSet(p.settings.OperatorRoleValues)
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		if normalized == RoleOwner {
			return RoleAdmin
		}
		if normalized == RoleAdmin || adminSet[normalized] {
			return RoleAdmin
		}
		if normalized == RoleOperator || operatorSet[normalized] {
			return RoleOperator
		}
		if normalized == RoleViewer {
			return RoleViewer
		}
	}
	return p.settings.DefaultRole
}

func normalizeRoleSet(raw []string) map[string]bool {
	out := make(map[string]bool, len(raw))
	for _, value := range raw {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == RoleOwner {
			normalized = RoleAdmin
		}
		if normalized != "" {
			out[normalized] = true
		}
	}
	return out
}

func claimString(claims map[string]any, key string) string {
	if claims == nil {
		return ""
	}
	if value, ok := claims[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func claimValues(value any) []string {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}
		return []string{trimmed}
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			trimmed := strings.TrimSpace(item)
			if trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				trimmed := strings.TrimSpace(text)
				if trimmed != "" {
					out = append(out, trimmed)
				}
			}
		}
		return out
	default:
		return nil
	}
}

func (p *OIDCProvider) SupportedClaims() []string {
	if p == nil {
		return nil
	}
	claims := []string{"sub", "preferred_username", "email", "name", p.settings.RoleClaim, "groups"}
	sort.Strings(claims)
	return claims
}
