package main

import (
	"context"
	"log"
	"strings"

	"github.com/labtether/labtether/internal/auth"
)

func loadOIDCProviderFromEnv(ctx context.Context) (*auth.OIDCProvider, bool, error) {
	issuerURL := strings.TrimSpace(envOrDefault("LABTETHER_OIDC_ISSUER_URL", ""))
	clientID := strings.TrimSpace(envOrDefault("LABTETHER_OIDC_CLIENT_ID", ""))
	enabled := envOrDefaultBool("LABTETHER_OIDC_ENABLED", issuerURL != "" || clientID != "")
	if !enabled {
		return nil, false, nil
	}

	settings := auth.OIDCSettings{
		Enabled:            true,
		IssuerURL:          issuerURL,
		ClientID:           strings.TrimSpace(envOrDefault("LABTETHER_OIDC_CLIENT_ID", "")),
		ClientSecret:       strings.TrimSpace(envOrDefault("LABTETHER_OIDC_CLIENT_SECRET", "")),
		Scopes:             parseCSVTokens(envOrDefault("LABTETHER_OIDC_SCOPES", "openid,profile,email")),
		RoleClaim:          strings.TrimSpace(envOrDefault("LABTETHER_OIDC_ROLE_CLAIM", "labtether_role")),
		DisplayName:        strings.TrimSpace(envOrDefault("LABTETHER_OIDC_DISPLAY_NAME", "Single Sign-On")),
		DefaultRole:        strings.TrimSpace(envOrDefault("LABTETHER_OIDC_DEFAULT_ROLE", auth.RoleViewer)),
		AdminRoleValues:    parseCSVTokens(envOrDefault("LABTETHER_OIDC_ADMIN_ROLES", "admin")),
		OperatorRoleValues: parseCSVTokens(envOrDefault("LABTETHER_OIDC_OPERATOR_ROLES", "operator")),
	}
	provider, err := auth.NewOIDCProvider(ctx, settings)
	if err != nil {
		return nil, false, err
	}
	autoProvision := envOrDefaultBool("LABTETHER_OIDC_AUTO_PROVISION", true)
	log.Printf("labtether auth: OIDC enabled (issuer=%s, auto_provision=%t)", provider.IssuerURL(), autoProvision)
	return provider, autoProvision, nil
}

func parseCSVTokens(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t' || r == ' '
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
