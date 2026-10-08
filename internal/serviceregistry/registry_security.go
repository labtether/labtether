package serviceregistry

// knownSecurityServices owns the security catalog.
func knownSecurityServices() map[string]KnownService {
	return map[string]KnownService{
		"vaultwarden": {
			Key: "vaultwarden", Name: "Vaultwarden", Category: CatSecurity, IconKey: "vaultwarden",
			DefaultPort: 8080, HealthPath: "/alive",
			DockerImages: []string{"vaultwarden/server"},
			Description:  "Lightweight Bitwarden-compatible password manager server",
		},
		"authelia": {
			Key: "authelia", Name: "Authelia", Category: CatSecurity, IconKey: "authelia",
			DefaultPort: 9091, HealthPath: "/api/health",
			DockerImages: []string{"authelia/authelia"},
			Description:  "Authentication and authorization server with SSO and 2FA",
		},
		"crowdsec": {
			Key: "crowdsec", Name: "CrowdSec", Category: CatSecurity, IconKey: "crowdsec",
			DefaultPort: 8080, HealthPath: "/v1/decisions",
			DockerImages: []string{"crowdsecurity/crowdsec"},
			Description:  "Collaborative intrusion prevention system",
		},
		"authentik": {
			Key: "authentik", Name: "Authentik", Category: CatSecurity, IconKey: "authentik",
			DefaultPort: 9000, HealthPath: "/api/v3/root/config/",
			DockerImages: []string{"ghcr.io/goauthentik/server"},
			Description:  "Flexible and versatile identity provider",
		},
		"keycloak": {
			Key: "keycloak", Name: "Keycloak", Category: CatSecurity, IconKey: "keycloak",
			DefaultPort: 8080, HealthPath: "/health",
			DockerImages: []string{"quay.io/keycloak/keycloak"},
			Description:  "Enterprise-grade identity and access management",
		},
		"oauth2-proxy": {
			Key: "oauth2-proxy", Name: "OAuth2 Proxy", Category: CatSecurity, IconKey: "oauth2-proxy",
			DefaultPort: 4180, HealthPath: "/ping",
			DockerImages: []string{"quay.io/oauth2-proxy/oauth2-proxy"},
			Description:  "Reverse proxy providing OAuth2/OIDC authentication",
		},
		"lldap": {
			Key: "lldap", Name: "LLDAP", Category: CatSecurity, IconKey: "lldap",
			DefaultPort: 17170, HealthPath: "/",
			DockerImages: []string{"lldap/lldap"},
			Description:  "Lightweight LDAP server for user management",
		},
		"zitadel": {
			Key: "zitadel", Name: "ZITADEL", Category: CatSecurity, IconKey: "zitadel",
			DefaultPort: 8080, HealthPath: "/debug/healthz",
			DockerImages: []string{"ghcr.io/zitadel/zitadel"},
			Description:  "Cloud-native identity and access management platform",
		},
	}
}
