package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
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
}

type OIDCIdentity struct {
	Subject           string         `json:"subject"`
	Email             string         `json:"email,omitempty"`
	Name              string         `json:"name,omitempty"`
	PreferredUsername string         `json:"preferred_username,omitempty"`
	Role              string         `json:"role"`
	Claims            map[string]any `json:"claims,omitempty"`
}

type OIDCProvider struct {
	settings OIDCSettings
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
}

func NewOIDCProvider(ctx context.Context, settings OIDCSettings) (*OIDCProvider, error) {
	if !settings.Enabled {
		return nil, nil
	}
	settings.IssuerURL = strings.TrimSpace(settings.IssuerURL)
	settings.ClientID = strings.TrimSpace(settings.ClientID)
	settings.ClientSecret = strings.TrimSpace(settings.ClientSecret)
	if settings.IssuerURL == "" {
		return nil, errors.New("oidc issuer url is required")
	}
	if settings.ClientID == "" {
		return nil, errors.New("oidc client id is required")
	}
	if settings.ClientSecret == "" {
		return nil, errors.New("oidc client secret is required")
	}
	if _, err := url.ParseRequestURI(settings.IssuerURL); err != nil {
		return nil, fmt.Errorf("invalid oidc issuer url: %w", err)
	}
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

	provider, err := oidc.NewProvider(ctx, settings.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("initialize oidc provider: %w", err)
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: settings.ClientID})

	return &OIDCProvider{settings: settings, provider: provider, verifier: verifier}, nil
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

func (p *OIDCProvider) ExchangeCode(ctx context.Context, code, nonce, redirectURI string) (OIDCIdentity, error) {
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
	token, err := cfg.Exchange(ctx, code)
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
		Subject:           strings.TrimSpace(idToken.Subject),
		Email:             claimString(claims, "email"),
		Name:              claimString(claims, "name"),
		PreferredUsername: claimString(claims, "preferred_username"),
		Claims:            claims,
		Role:              p.resolveRole(claims),
	}
	if identity.Subject == "" {
		return OIDCIdentity{}, errors.New("oidc subject claim is missing")
	}
	return identity, nil
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
