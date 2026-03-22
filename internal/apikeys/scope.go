package apikeys

import (
	"fmt"
	"strings"
)

var knownScopeCategories = map[string]bool{
	"assets": true, "files": true, "services": true, "processes": true,
	"docker": true, "connectors": true, "groups": true, "alerts": true,
	"shell": true, "network": true, "disks": true, "packages": true,
	"cron": true, "users": true, "homeassistant": true, "agents": true,
	"collectors": true, "web-services": true, "webhooks": true,
	"schedules": true, "actions": true, "events": true, "search": true,
	"metrics": true, "hub": true, "failover": true, "bulk": true,
	"terminal": true, "dead-letters": true, "logs": true,
	"credentials": true, "audit": true, "settings": true,
	"discovery": true, "topology": true, "updates": true,
	"incidents": true, "notifications": true,
}

func ScopeAllows(granted []string, required string) bool {
	if required == "" {
		return false
	}
	for _, g := range granted {
		if g == "*" {
			return true
		}
		if g == required {
			return true
		}
		if strings.HasSuffix(g, ":*") {
			category := strings.TrimSuffix(g, ":*")
			if strings.HasPrefix(required, category+":") {
				return true
			}
		}
		// Bare category name (e.g. "assets") matches any scope in that category.
		if !strings.Contains(g, ":") && strings.HasPrefix(required, g+":") {
			return true
		}
	}
	return false
}

func AssetAllowed(allowedAssets []string, assetID string) bool {
	if len(allowedAssets) == 0 {
		return true
	}
	for _, a := range allowedAssets {
		if a == assetID {
			return true
		}
	}
	return false
}

func ValidateScopes(scopes []string) error {
	if len(scopes) == 0 {
		return fmt.Errorf("at least one scope is required")
	}
	for _, s := range scopes {
		if s == "*" {
			continue
		}
		if !strings.Contains(s, ":") {
			if !knownScopeCategories[s] {
				return fmt.Errorf("unknown scope: %q", s)
			}
			continue
		}
		parts := strings.SplitN(s, ":", 2)
		if !knownScopeCategories[parts[0]] {
			return fmt.Errorf("unknown scope category: %q", parts[0])
		}
	}
	return nil
}
