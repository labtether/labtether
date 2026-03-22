package main

import (
	"context"
	"net/http"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/servicehttp"
)

// The context key type and constants now live in internal/apiv2.
// These thin aliases keep existing call sites in cmd/labtether/ compiling
// without modification while the canonical implementations are in the shared
// package so that handler packages in internal/hubapi/ can also use them.

func contextWithUserID(ctx context.Context, userID string) context.Context {
	return apiv2.ContextWithUserID(ctx, userID)
}

func contextWithUserRole(ctx context.Context, role string) context.Context {
	return apiv2.ContextWithUserRole(ctx, role)
}

func contextWithPrincipal(ctx context.Context, userID, role string) context.Context {
	return apiv2.ContextWithPrincipal(ctx, userID, role)
}

func userIDFromContext(ctx context.Context) string {
	return apiv2.UserIDFromContext(ctx)
}

func userRoleFromContext(ctx context.Context) string {
	return apiv2.UserRoleFromContext(ctx)
}

func principalActorID(ctx context.Context) string {
	return apiv2.PrincipalActorID(ctx)
}

func contextWithScopes(ctx context.Context, scopes []string) context.Context {
	return apiv2.ContextWithScopes(ctx, scopes)
}

func scopesFromContext(ctx context.Context) []string {
	return apiv2.ScopesFromContext(ctx)
}

func contextWithAllowedAssets(ctx context.Context, assets []string) context.Context {
	return apiv2.ContextWithAllowedAssets(ctx, assets)
}

func allowedAssetsFromContext(ctx context.Context) []string {
	return apiv2.AllowedAssetsFromContext(ctx)
}

func contextWithAPIKeyID(ctx context.Context, keyID string) context.Context {
	return apiv2.ContextWithAPIKeyID(ctx, keyID)
}

func apiKeyIDFromContext(ctx context.Context) string {
	return apiv2.APIKeyIDFromContext(ctx)
}

// requireAdminAuth checks that the request has admin privileges.
// If not, it writes a 403 Forbidden response and returns false.
// Use as an early-return guard in destructive action handlers:
//
//	if !s.requireAdminAuth(w, r) {
//	    return
//	}
func (s *apiServer) requireAdminAuth(w http.ResponseWriter, r *http.Request) bool {
	if !auth.HasAdminPrivileges(userRoleFromContext(r.Context())) {
		servicehttp.WriteError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}
